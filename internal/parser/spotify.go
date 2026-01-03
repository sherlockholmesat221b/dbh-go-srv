package parser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"dbh-go-srv/internal/models"
    "dbh-go-srv/internal/dab"
	"github.com/zmb3/spotify/v2"
)

// guestTokenTransport is the missing struct definition
type guestTokenTransport struct {
	Token string
}

// RoundTrip injects the guest token into every Spotify API request
func (t *guestTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+t.Token)
	// Some Spotify endpoints require a specific User-Agent even for API calls
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	return http.DefaultTransport.RoundTrip(req)
}

// ParseSpotify handles public resources using a guest token
func ParseSpotify(url string) ([]models.Track, string, error) {
	token, err := getGuestToken()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get guest token: %w", err)
	}

	httpClient := &http.Client{
		Transport: &guestTokenTransport{Token: token},
		Timeout:   15 * time.Second,
	}
	client := spotify.New(httpClient)

	p := &SpotifyParser{client: client}
	return p.extract(url)
}


func getGuestToken() (string, error) {
	req, _ := http.NewRequest("GET", "https://open.spotify.com/get_access_token?reason=transport&productType=web_player", nil)
	
    // MUST use the exact UA defined in dab package
	req.Header.Set("User-Agent", dab.UserAgent)
	req.Header.Set("Referer", "https://open.spotify.com/")

	resp, err := http.DefaultClient.Do(req)
	if err != nil { return "", err }
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("spotify token service returned status %d", resp.StatusCode)
	}

	var res struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", fmt.Errorf("failed to parse guest token: %w", err)
	}
	return res.AccessToken, nil
}

type SpotifyParser struct {
	client *spotify.Client
}

func (p *SpotifyParser) extract(url string) ([]models.Track, string, error) {
	ctx := context.Background()
	parts := strings.Split(url, "/")
	rawID := strings.Split(parts[len(parts)-1], "?")[0]
	id := spotify.ID(rawID)

	switch {
	case strings.Contains(url, "playlist"):
		res, err := p.client.GetPlaylist(ctx, id)
		if err != nil {
			return nil, "", err
		}
		var tracks []models.Track
		for _, item := range res.Tracks.Tracks {
			tracks = append(tracks, p.transform(item.Track))
		}
		return tracks, res.Name, nil

	case strings.Contains(url, "album"):
		res, err := p.client.GetAlbum(ctx, id)
		if err != nil {
			return nil, "", err
		}
		var tracks []models.Track
		for _, item := range res.Tracks.Tracks {
			tracks = append(tracks, models.Track{
				Title:    item.Name,
				Artist:   item.Artists[0].Name,
				SourceID: string(item.ID),
			})
		}
		return tracks, res.Name, nil

	case strings.Contains(url, "track"):
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
		SourceID: string(st.ID),
	}
}
