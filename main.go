package main

import (
	"context"
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
	_ "github.com/mattn/go-sqlite3"

	"dbh-go-srv/internal/dab"
	"dbh-go-srv/internal/database"
	"dbh-go-srv/internal/matcher"
	"dbh-go-srv/internal/models"
	"dbh-go-srv/internal/parser"
)

/* =========================
   Recovery Middleware
   ========================= */

func RecoveryMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC: %v\n%s", err, debug.Stack())
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next(w, r)
	}
}

/* =========================
   Types
   ========================= */

type ConversionRequest struct {
	URL          string `json:"url"`
	Type         string `json:"type"`
	MatchingMode string `json:"matching_mode"`
}

/* =========================
   SSE Helpers
   ========================= */

func setupSSE(w http.ResponseWriter) (http.Flusher, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming unsupported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	return flusher, nil
}

func sendEvent(w http.ResponseWriter, flusher http.Flusher, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		log.Println("SSE marshal error:", err)
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", b)
	flusher.Flush()
}

/* =========================
   Handler
   ========================= */

func handleConvert(db *sql.DB, w http.ResponseWriter, r *http.Request) {
	// ---- CORS Preflight ----
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-DAB-Token")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	flusher, err := setupSSE(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	send := func(v any) { sendEvent(w, flusher, v) }
	fail := func(msg string) {
		send(map[string]string{"status": "error", "message": msg})
	}

	var (
		tracks     []models.Track
		sourceName string
		reqType    string
		matchMode  string
	)

	/* ---------- Parse Request ---------- */

	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			fail("Invalid multipart form")
			return
		}

		reqType = r.FormValue("type")
		matchMode = r.FormValue("matching_mode")

		if reqType != "csv" {
			fail("multipart only supported for type=csv")
			return
		}

		tracks, sourceName, err = parser.ParseCSV(r)
		if err != nil {
			fail("CSV parse failed: " + err.Error())
			return
		}

	} else {
		var req ConversionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail("Invalid JSON body")
			return
		}

		reqType = req.Type
		matchMode = req.MatchingMode

		parsedURL, err := url.Parse(req.URL)
		if err != nil || parsedURL.Host == "" {
			fail("Invalid URL")
			return
		}

		send(map[string]string{
			"status":  "extracting",
			"message": "Parsing " + reqType,
		})

		switch reqType {
		case "spotify":
			if !strings.Contains(parsedURL.Host, "spotify.com") &&
				!strings.Contains(parsedURL.Host, "googleusercontent.com") {
				fail("Invalid Spotify URL")
				return
			}
			tracks, sourceName, err = parser.ParseSpotify(req.URL)

		case "youtube":
			if !strings.Contains(parsedURL.Host, "youtube.com") &&
				!strings.Contains(parsedURL.Host, "youtu.be") {
				fail("Invalid YouTube URL")
				return
			}
			tracks, sourceName, err = parser.ParseYouTube(req.URL)

		default:
			fail("Unsupported source type")
			return
		}

		if err != nil {
			fail("Extraction failed: " + err.Error())
			return
		}
	}

	if len(tracks) == 0 {
		fail("No tracks found")
		return
	}

	/* ---------- Auth ---------- */

	token := r.Header.Get("X-DAB-Token")
	if token == "" {
		fail("Missing X-DAB-Token")
		return
	}

	client := dab.GetClient(token)

	send(map[string]string{"status": "info", "message": "Verifying session..."})

	userID, err := client.ValidateToken()
	if err != nil {
		fail("Auth failed: " + err.Error())
		return
	}

	/* ---------- Matching ---------- */

	results := make([]models.MatchResult, 0, len(tracks))

	for i, t := range tracks {
		select {
		case <-ctx.Done():
			log.Println("Client disconnected")
			return
		default:
		}

		res := matcher.MatchTrack(db, client, t, matchMode)
		results = append(results, *res)

		send(map[string]any{
			"status": "processing",
			"index":  i + 1,
			"total":  len(tracks),
			"result": res,
		})
	}

	/* ---------- Final ---------- */

	send(map[string]any{
		"status": "complete",
		"meta": map[string]any{
			"user_id":     userID,
			"source_name": sourceName,
			"timestamp":   time.Now().Format(time.RFC3339),
		},
		"tracks": results,
	})
}

/* =========================
   Main
   ========================= */

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env found, using environment")
	}

	dbPath := "./data/registry.db"
	_ = os.MkdirAll(filepath.Dir(dbPath), 0755)

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := database.InitDatabase(db); err != nil {
		log.Fatal(err)
	}

	http.HandleFunc(
		"/api/v1/convert",
		RecoveryMiddleware(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost && r.Method != http.MethodOptions {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			handleConvert(db, w, r)
		}),
	)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("DBH Matcher Engine listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}