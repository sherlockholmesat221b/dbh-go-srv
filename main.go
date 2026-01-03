package main

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	// Enable streaming
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Helper for structured progress/logs
	sendProgress := func(data interface{}) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", string(b))
		flusher.Flush()
	}

	// Helper for errors that matches the structured output
	sendError := func(msg string) {
		sendProgress(map[string]string{"status": "error", "message": msg})
	}

	// 1. Decode Request
	var req ConversionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError("Invalid request JSON: " + err.Error())
		return
	}

	sendProgress(map[string]string{"status": "extracting", "message": "Parsing " + req.Type})

	// 2. Auth check
	token := r.Header.Get("X-DAB-Token")
	if token == "" {
		sendError("Missing X-DAB-Token header")
		return
	}
	client := dab.GetClient(token)
	sendProgress(map[string]string{"status": "info", "message": "✓ Authenticated with DAB API"})

	// 3. Extract Tracks from Source
	var tracks []models.Track
	var sourceName string
	var err error

	switch req.Type {
	case "spotify":
		tracks, sourceName, err = parser.ParseSpotify(req.URL)
	case "youtube":
		tracks, sourceName, err = parser.ParseYouTube(req.URL)
	default:
		sendError("Unsupported source type: " + req.Type)
		return
	}

	if err != nil {
		sendError(fmt.Sprintf("Extraction failed: %v", err))
		return
	}

	sendProgress(map[string]interface{}{
		"status": "extracted",
		"count":  len(tracks),
		"source": sourceName,
	})

	// 4. Create Library
	if sourceName == "" {
		sourceName = "DABHounds " + time.Now().Format("2006-01-02 15:04")
	}

	sendProgress(map[string]string{"status": "info", "message": "Creating DAB library..."})
	libID, err := client.CreateLibrary(sourceName)
	if err != nil {
		sendError(fmt.Sprintf("Library creation failed: %v", err))
		return
	}
	sendProgress(map[string]string{"status": "info", "message": "✓ Library created (ID: " + libID + ")"})

	// 5. Match and Add (Detailed Loop)
	var matchedTracks []models.MatchResult
	for i, t := range tracks {
		res := matcher.MatchTrack(client, t, req.MatchingMode)
		matchedTracks = append(matchedTracks, *res)

		status := "NOT_FOUND"
		if res.MatchStatus == "FOUND" && res.DabTrackID != nil {
			err := client.AddTrackToLibrary(libID, *res)
			if err == nil {
				status = "ADDED"
			} else {
				status = "DAB_ERROR"
			}
		}

		// Detailed per-track progress
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

	// 6. Final Report
	report := models.Report{
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
	port := ":8080"
	fmt.Println("Server starting on " + port)
	if err := http.ListenAndServe(port, nil); err != nil {
		panic(err)
	}
}
