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

	sendProgress := func(data interface{}) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", string(b))
		flusher.Flush()
	}

	sendError := func(msg string) {
		sendProgress(map[string]string{"status": "error", "message": msg})
	}

	// 1. Decode
	var req ConversionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError("Invalid request JSON")
		return
	}

	// 2. Auth
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

	// 3. Extract
	parsedURL, _ := url.ParseRequestURI(req.URL)
	sendProgress(map[string]string{"status": "extracting", "message": "Parsing " + req.Type})
	
	var tracks []models.Track
	var sourceName string

	switch req.Type {
	case "spotify":
		if !strings.Contains(parsedURL.Host, "spotify.com") && !strings.Contains(parsedURL.Host, "googleusercontent.com") {
			sendError("Invalid Spotify URL")
			return
		}
		tracks, sourceName, err = parser.ParseSpotify(req.URL)
	case "youtube":
		if !strings.Contains(parsedURL.Host, "youtube.com") && !strings.Contains(parsedURL.Host, "youtu.be") {
			sendError("Invalid YouTube URL")
			return
		}
		tracks, sourceName, err = parser.ParseYouTube(req.URL)
	default:
		sendError("Unsupported type")
		return
	}

	if err != nil || len(tracks) == 0 {
		sendError("Extraction failed")
		return
	}

	// 4. Create Lib
	libID, err := client.CreateLibrary(sourceName)
	if err != nil {
		sendError("Lib creation failed")
		return
	}

	// 5. Match & Add
	var matchedTracks []models.MatchResult
	for i, t := range tracks {
		res := matcher.MatchTrack(db, client, t, req.MatchingMode)
		
		status := "NOT_FOUND"
		if res.MatchStatus == "FOUND" && res.DabTrackID != nil {
			if err := client.AddTrackToLibrary(libID, *res.DabTrackID); err == nil {
				status = "ADDED"
			}
		}

		matchedTracks = append(matchedTracks, *res)
		sendProgress(map[string]interface{}{
			"status": "processing",
			"index":  i + 1,
			"total":  len(tracks),
			"track":  map[string]string{"title": t.Title, "result": status},
		})
	}

	// 6. Final Report
	report := models.Report{
		UserID:       userID,
		Library:      models.LibraryInfo{ID: libID, Name: sourceName},
		Source:       models.SourceInfo{Type: req.Type, URL: req.URL},
		MatchingMode: req.MatchingMode,
		Timestamp:    time.Now().Format(time.RFC3339),
		Tracks:       matchedTracks,
	}
	sendProgress(map[string]interface{}{"status": "complete", "report": report})
}

func main() {
	dbPath := "./data/registry.db"
	os.MkdirAll(filepath.Dir(dbPath), 0755)

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := database.InitDatabase(db); err != nil {
		log.Fatal(err)
	}

	// Wrap the handler with the Recovery Middleware
	http.HandleFunc("/api/v1/convert", RecoveryMiddleware(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleConvert(db, w, r)
	}))

	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
