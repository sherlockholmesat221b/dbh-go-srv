package parser

import (
	"context"
	"fmt"
	"os"
	"strings"

	"dbh-go-srv/internal/models"
	"github.com/zmb3/spotify/v2"
	"github.com/zmb3/spotify/v2/auth"
    "golang.org/x/oauth2/clientcredentials"
)

func ParseSpotify(url string) ([]models.Track, string, error) {
	ctx := context.Background()

	// 1. Fetch credentials from Environment Variables
	clientID := os.Getenv("SPOTIFY_ID")
	clientSecret := os.Getenv("SPOTIFY_SECRET")

	if clientID == "" || clientSecret == "" {
		return nil, "", fmt.Errorf("spotify credentials missing (SPOTIFY_ID/SPOTIFY_SECRET)")
	}

	// 2. Setup Official Auth
	config := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     spotifyauth.TokenURL,
	}

	// The library automatically handles token refreshing
	httpClient := config.Client(ctx)
	client := spotify.New(httpClient)

	p := &SpotifyParser{client: client}
	return p.extract(url)
}

type SpotifyParser struct {
	client *spotify.Client
}

func (p *SpotifyParser) extract(url string) ([]models.Track, string, error) {
	ctx := context.Background()
	
	// Better parsing using the official library's logic
	parts := strings.Split(url, "/")
	if len(parts) < 2 {
		return nil, "", fmt.Errorf("invalid spotify URL")
	}
	rawID := strings.Split(parts[len(parts)-1], "?")[0]
	id := spotify.ID(rawID)

	switch {
	case strings.Contains(url, "/playlist/"):
		res, err := p.client.GetPlaylist(ctx, id)
		if err != nil {
			return nil, "", err
		}
		var tracks []models.Track
		for _, item := range res.Tracks.Tracks {
			tracks = append(tracks, p.transform(item.Track))
		}
		return tracks, res.Name, nil

	case strings.Contains(url, "/album/"):
		res, err := p.client.GetAlbum(ctx, id)
		if err != nil {
			return nil, "", err
		}
		var tracks []models.Track
		for _, item := range res.Tracks.Tracks {
			// GetAlbum returns SimpleTracks, so we map them manually
			tracks = append(tracks, models.Track{
				Title:    item.Name,
				Artist:   item.Artists[0].Name,
				Album:    res.Name,
				SourceID: string(item.ID),
			})
		}
		return tracks, res.Name, nil

	case strings.Contains(url, "/track/"):
		res, err := p.client.GetTrack(ctx, id)
		if err != nil {
			return nil, "", err
		}
		return []models.Track{p.transform(*res)}, res.Name, nil

	default:
		return nil, "", fmt.Errorf("unsupported Spotify URL type")
	}
}

func (p *SpotifyParser) transform(st spotify.FullTrack) models.Track {
	var artists []string
	for _, a := range st.Artists {
		artists = append(artists, a.Name)
	}
	return models.Track{
		Title:    st.Name,
		Artist:   strings.Join(artists, ", "),
		Album:    st.Album.Name,
		ISRC:     st.ExternalIDs["isrc"],
        Type:     "spotify",
		SourceID: string(st.ID),
	}
}
