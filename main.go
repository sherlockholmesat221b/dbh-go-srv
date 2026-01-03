package main

import (
	"encoding/json"
    "strings"
	"fmt"
	"net/http"
    "net/url"
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

	// 1. Decode Request
    var req ConversionRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        sendError("Invalid request JSON")
        return
    }

    // Security: Validate the URL format
    parsedURL, err := url.ParseRequestURI(req.URL)
    if err != nil || !strings.Contains(parsedURL.Host, "youtube.com") && !strings.Contains(parsedURL.Host, "youtu.be") {
        if req.Type == "youtube" {
        sendError("Invalid YouTube URL")
        return
        }
    }

    // Only use the validated string version
    cleanURL := parsedURL.String()

	// 2. Auth check
	token := r.Header.Get("X-DAB-Token")
	if token == "" {
		sendError("Missing X-DAB-Token header")
		return
	}
	client := dab.GetClient(token)

	// 3. Extract Tracks
	sendProgress(map[string]string{"status": "extracting", "message": "Parsing " + req.Type})
	var tracks []models.Track
	var sourceName string

	switch req.Type {
	case "spotify":
		tracks, sourceName, err = parser.ParseSpotify(req.URL)
	case "youtube":
		tracks, sourceName, err = parser.ParseYouTube(cleanURL)
	default:
		sendError("Unsupported source type")
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

	libID, err := client.CreateLibrary(sourceName)
	if err != nil {
		sendError(fmt.Sprintf("Library creation failed: %v", err))
		return
	}

	// 5. Match and Add
	var matchedTracks []models.MatchResult
	for i, t := range tracks {
		res := matcher.MatchTrack(client, t, req.MatchingMode)
		matchedTracks = append(matchedTracks, *res)

		currentStatus := "NOT_FOUND"

		if res.MatchStatus == "FOUND" && res.RawTrack != nil {
			// Type assert the interface back to DabTrack
			if dt, ok := res.RawTrack.(*dab.DabTrack); ok {
				err := client.AddTrackToLibrary(libID, *dt)
				if err == nil {
					currentStatus = "ADDED"
				} else {
					currentStatus = "DAB_ERROR"
				}
			}
		}

		sendProgress(map[string]interface{}{
			"status": "processing",
			"index":  i + 1,
			"total":  len(tracks),
			"track": map[string]string{
				"title":  t.Title,
				"artist": t.Artist,
				"result": currentStatus,
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
	fmt.Println("Server starting on :8080")
	http.ListenAndServe(":8080", nil)
}
