package parser

import (
	"fmt"

	"dbh-go-srv/internal/models"
	"github.com/kkdai/youtube/v2"
)

func ParseYouTube(url string) ([]models.Track, string, error) {
	client := youtube.Client{}

	// 1. Try to parse as a playlist first
	playlist, err := client.GetPlaylist(url)
	if err == nil {
		var tracks []models.Track
		
		for _, entry := range playlist.Videos {
			artist, title := NormalizeYTTitle(entry.Title, entry.Author)
			
			tracks = append(tracks, models.Track{
				Title:    title,
				Artist:   artist,
				SourceID: entry.ID,
				Type:     "youtube", // Set type for registry
			})
		}
		return tracks, playlist.Title, nil
	}

	// 2. Fallback: Parse as a single video
	video, err := client.GetVideo(url)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse YouTube URL: %w", err)
	}

	artist, title := NormalizeYTTitle(video.Title, video.Author)
	tracks := []models.Track{
		{
			Title:    title,
			Artist:   artist,
			SourceID: video.ID,
			Type:     "youtube", // Set type for registry
		},
	}

	return tracks, video.Title, nil
}
