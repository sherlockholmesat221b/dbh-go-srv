package parser

import (
	"encoding/json"
	"os/exec"
    "strings"
    
	"dbh-go-srv/internal/models"
)

type YtDlpEntry struct {
	Title    string `json:"title"`
	Uploader string `json:"uploader"`
	ID       string `json:"id"`
	ISRC     string `json:"isrc"`
}

func ParseYouTube(url string) ([]models.Track, string, error) {
	// Execute yt-dlp to get flat playlist info
	cmd := exec.Command("yt-dlp", "--dump-json", "--flat-playlist", "--no-warnings", url)
	output, err := cmd.Output()
	if err != nil {
		return nil, "", err
	}

	// yt-dlp outputs one JSON object per line for playlists
	lines := strings.Split(string(output), "\n")
	var tracks []models.Track
	var playlistTitle string

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry YtDlpEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		artist, title := NormalizeYTTitle(entry.Title, entry.Uploader)
		
		tracks = append(tracks, models.Track{
			Title:    title,
			Artist:   artist,
			ISRC:     entry.ISRC,
			SourceID: entry.ID,
		})
	}

	return tracks, playlistTitle, nil
}
