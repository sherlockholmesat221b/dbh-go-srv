package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
    "github.com/joho/godotenv"

	"dbh-go-srv/internal/dab"
	"dbh-go-srv/internal/database"
	"dbh-go-srv/internal/matcher"
	"dbh-go-srv/internal/models"
	"dbh-go-srv/internal/parser"

	_ "github.com/mattn/go-sqlite3"
)

// RecoveryMiddleware catches panics so the server stays alive
func RecoveryMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC RECOVERED: %v\n%s", err, debug.Stack())
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next(w, r)
	}
}

type ConversionRequest struct {
	URL          string `json:"url"`
	Type         string `json:"type"`
	MatchingMode string `json:"matching_mode"`
}

func handleConvert(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	// SSE Setup
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*") // Adjust for production
	contentType := r.Header.Get("Content-Type")

	sendProgress := func(data interface{}) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", string(b))
		flusher.Flush()
	}

	sendError := func(msg string) {
		sendProgress(map[string]string{"status": "error", "message": msg})
	}

	// ---- Shared variables ----
	var (
		tracks     []models.Track
		sourceName string
		reqType    string
		matchMode  string
		err        error
	)

	// ---- CSV (multipart) ----
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			sendError("Invalid multipart form")
			return
		}

		reqType = r.FormValue("type")
		matchMode = r.FormValue("matching_mode")

		if reqType != "csv" {
			sendError("multipart requests only supported for type=csv")
			return
		}

		tracks, sourceName, err = parser.ParseCSV(r)
		if err != nil {
			sendError("CSV parse failed: " + err.Error())
			return
		}

	// ---- JSON (Spotify / YouTube) ----
	} else {
		var req ConversionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendError("Invalid request JSON")
			return
		}

		reqType = req.Type
		matchMode = req.MatchingMode

		parsedURL, err := url.ParseRequestURI(req.URL)
		if err != nil {
			sendError("Invalid URL format")
			return
		}

		sendProgress(map[string]string{
			"status":  "extracting",
			"message": "Parsing " + reqType,
		})

		switch reqType {
		case "spotify":
			if !strings.Contains(parsedURL.Host, "spotify.com") &&
				!strings.Contains(parsedURL.Host, "googleusercontent.com") {
				sendError("Invalid Spotify URL")
				return
			}
			tracks, sourceName, err = parser.ParseSpotify(req.URL)

		case "youtube":
			if !strings.Contains(parsedURL.Host, "youtube.com") &&
				!strings.Contains(parsedURL.Host, "youtu.be") {
				sendError("Invalid YouTube URL")
				return
			}
			tracks, sourceName, err = parser.ParseYouTube(req.URL)

		default:
			sendError("Unsupported source type: " + reqType)
			return
		}

		if err != nil {
			sendError("Extraction failed: " + err.Error())
			return
		}
	}

	// ---- Post-parsing validation ----
	if len(tracks) == 0 {
		sendError("No tracks found")
		return
	}

	// ---- Auth ----
	token := r.Header.Get("X-DAB-Token")
	if token == "" {
		sendError("Missing X-DAB-Token")
		return
	}
	client := dab.GetClient(token)

	sendProgress(map[string]string{"status": "info", "message": "Verifying session..."})
	userID, err := client.ValidateToken()
	if err != nil {
		sendError("Auth failed: " + err.Error())
		return
	}

	// ---- Track matching ----
	var results []models.MatchResult
	for i, t := range tracks {
		res := matcher.MatchTrack(db, client, t, matchMode)
		results = append(results, *res)

		sendProgress(map[string]interface{}{
			"status": "processing",
			"index":  i + 1,
			"total":  len(tracks),
			"result": res,
		})
	}

	// ---- Final report ----
	report := map[string]interface{}{
		"status": "complete",
		"meta": map[string]interface{}{
			"user_id":     userID,
			"source_name": sourceName,
			"source_url":  "", // CSV has no source URL
			"timestamp":   time.Now().Format(time.RFC3339),
		},
		"tracks": results,
	}
	sendProgress(report)
}

func main() {
    // 1. Load .env file at the absolute start
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: No .env file found; using system environment variables")
	}
	dbPath := "./data/registry.db"
	_ = os.MkdirAll(filepath.Dir(dbPath), 0755)

	// Open DB with WAL mode for better concurrency during matches
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := database.InitDatabase(db); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	http.HandleFunc("/api/v1/convert", RecoveryMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleConvert(db, w, r)
	}))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("DBH Matcher Engine starting on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
