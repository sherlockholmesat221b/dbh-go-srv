package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"dbh-go-srv/internal/dab"
	"dbh-go-srv/internal/matcher"
	"dbh-go-srv/internal/models"
	"dbh-go-srv/internal/parser"
)

type ConversionRequest struct {
	URL          string `json:"url"`
	Type         string `json:"type"`          // spotify | youtube | csv
	MatchingMode string `json:"matching_mode"` // strict | lenient
}

func handleConvert(w http.ResponseWriter, r *http.Request) {
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

	// --- 1. DECODE REQUEST ---
	var req ConversionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError("Invalid request JSON")
		return
	}

	// --- 2. AUTHENTICATION & JWT VALIDATION (CRITICAL) ---
	token := r.Header.Get("X-DAB-Token")
	if token == "" {
		sendError("Missing X-DAB-Token header")
		return
	}
	client := dab.GetClient(token)

	// Validate the token and get the user info before any resource-heavy work
	sendProgress(map[string]string{"status": "info", "message": "Verifying DAB session..."})
	userID, err := client.ValidateToken()
	if err != nil {
		sendError("Authentication failed: " + err.Error())
		return
	}
	sendProgress(map[string]string{"status": "info", "message": "✓ Authenticated as User " + userID})

	// --- 3. URL VALIDATION ---
	parsedURL, err := url.ParseRequestURI(req.URL)
	if err != nil {
		sendError("Invalid URL format")
		return
	}
	cleanURL := parsedURL.String()

	// --- 4. EXTRACTION ---
	sendProgress(map[string]string{"status": "extracting", "message": "Parsing source: " + req.Type})
	var tracks []models.Track
	var sourceName string

	switch req.Type {
	case "spotify":
		if !strings.Contains(parsedURL.Host, "spotify.com") && !strings.Contains(parsedURL.Host, "open.spotify.com") {
            sendError("Invalid Spotify URL")
			return
		}
		tracks, sourceName, err = parser.ParseSpotify(cleanURL)
	case "youtube":
		if !strings.Contains(parsedURL.Host, "youtube.com") && !strings.Contains(parsedURL.Host, "youtu.be") {
			sendError("Invalid YouTube URL")
			return
		}
		tracks, sourceName, err = parser.ParseYouTube(cleanURL)
	default:
		sendError("Unsupported source type: " + req.Type)
		return
	}

	if err != nil || len(tracks) == 0 {
		sendError(fmt.Sprintf("Extraction failed or empty: %v", err))
		return
	}

	sendProgress(map[string]interface{}{
		"status": "extracted",
		"count":  len(tracks),
		"source": sourceName,
	})

	// --- 5. LIBRARY CREATION ---
	if sourceName == "" {
		sourceName = "DABHounds " + time.Now().Format("2006-01-02 15:04")
	}

	sendProgress(map[string]string{"status": "info", "message": "Creating DAB library..."})
	libID, err := client.CreateLibrary(sourceName)
	if err != nil {
		sendError("Library creation failed: " + err.Error())
		return
	}

	// --- 6. MATCHING AND ADDING ---
	var matchedTracks []models.MatchResult
	for i, t := range tracks {
		res := matcher.MatchTrack(client, t, req.MatchingMode)
		
		status := "NOT_FOUND"
		if res.MatchStatus == "FOUND" && res.RawTrack != nil {
			if dt, ok := res.RawTrack.(*dab.DabTrack); ok {
				err := client.AddTrackToLibrary(libID, *dt)
				if err == nil {
					status = "ADDED"
				} else {
					status = "DAB_ERROR"
				}
			}
		}

		matchedTracks = append(matchedTracks, *res)

		sendProgress(map[string]interface{}{
			"status": "processing",
			"index":  i + 1,
			"total":  len(tracks),
			"track": map[string]string{
				"title":  t.Title,
				"artist": t.Artist,
				"result": status,
			},
		})
	}

	// --- 7. FINAL REPORT (Includes UserID) ---
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
	http.HandleFunc("/api/v1/convert", handleConvert)
	fmt.Println("Server starting on :8080")
	http.ListenAndServe(":8080", nil)
}
