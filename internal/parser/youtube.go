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
		
		// The library returns a playlist object with a list of videos
		for _, entry := range playlist.Videos {
			// entry.Author is the Uploader
			artist, title := NormalizeYTTitle(entry.Title, entry.Author)
			
			tracks = append(tracks, models.Track{
				Title:    title,
				Artist:   artist,
				SourceID: entry.ID,
				// Note: Native Go libs rarely extract ISRC as easily as yt-dlp
				// because it requires deep metadata parsing.
			})
		}
		return tracks, playlist.Title, nil
	}

	// 2. Fallback: Parse as a single video if playlist parsing fails
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
		},
	}

	return tracks, video.Title, nil
}
