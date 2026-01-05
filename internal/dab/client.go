package dab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os" // Added missing import
	"time"

	"golang.org/x/time/rate"
)

const (
	DABAPIBase   = "https://dabmusic.xyz/api"
	QobuzAPIBase = "https://www.qobuz.com/api.json/0.2"
	UserAgent    = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/1337.0.0.0 Safari/537.36"
)

// DabTrack represents the unified track object
type DabTrack struct {
	ID           int         `json:"id"`
	Title        string      `json:"title"`
	Artist       string      `json:"artist"`
	ArtistID     int         `json:"artistId"`
	AlbumTitle   string      `json:"albumTitle"`
	AlbumCover   string      `json:"albumCover"`
	AlbumID      interface{} `json:"albumId"`
	ReleaseDate  string      `json:"releaseDate"`
	Genre        string      `json:"genre"`
	Duration     int         `json:"duration"`
	AudioQuality struct {
		SamplingRate float64 `json:"maximumSampleRate"`
		BitDepth     int     `json:"maximumBitDepth"`
		IsHiRes      bool    `json:"isHiRes"`
	} `json:"audioQuality"`
}

type Client struct {
	HTTPClient *http.Client
	Limiter    *rate.Limiter
	Token      string
	QobuzID    string // Added field to struct
}

// GetClient initializes the client with environment variables and rate limiting
func GetClient(token string) *Client {
	// Pull Qobuz App ID from env or fallback to your default
	qobuzID := os.Getenv("QOBUZ_APP_ID")
	if qobuzID == "" {
		qobuzID = "000000000" // Default fallback
	}

	return &Client{
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		// 1.5 requests per second rate limit for DAB
		Limiter: rate.NewLimiter(rate.Every(666*time.Millisecond), 1),
		Token:   token,
		QobuzID: qobuzID,
	}
}

// Do handles rate limiting, auth headers, and cookies for DAB API calls
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	c.Limiter.Wait(context.Background())

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Cookie", fmt.Sprintf("session=%s", c.Token))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://dabmusic.xyz/")
	req.Header.Set("Origin", "https://dabmusic.xyz")

	return c.HTTPClient.Do(req)
}

// Search orchestrates Qobuz-first matching with DAB fallback
func (c *Client) Search(query string) []DabTrack {
	// 1. Try Qobuz (Higher quality, no rate limit)
	qTracks, err := c.searchQobuz(query)
	if err == nil && len(qTracks) > 0 {
		return qTracks
	}

	// 2. Fallback to DAB (Internal search)
	return c.searchDab(query)
}

func (c *Client) searchQobuz(query string) ([]DabTrack, error) {
	searchURL := fmt.Sprintf("%s/track/search?query=%s&limit=5&app_id=%s", QobuzAPIBase, url.QueryEscape(query), c.QobuzID)
	req, _ := http.NewRequest("GET", searchURL, nil)
	req.Header.Set("X-App-Id", c.QobuzID)

	// Use HTTPClient directly for Qobuz to bypass DAB's rate limiter
	resp, err := c.HTTPClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qobuz failed")
	}
	defer resp.Body.Close()

	var qRes QobuzSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&qRes); err != nil {
		return nil, err
	}

	var converted []DabTrack
	for _, item := range qRes.Tracks.Items {
		dt := DabTrack{
			ID:         item.ID,
			Title:      item.Title,
			Artist:     item.Album.Artist.Name,
			ArtistID:   item.Album.Artist.ID,
			AlbumTitle: item.Album.Title,
			AlbumCover: item.Album.Image.Large,
			AlbumID:    item.Album.ID,
			Duration:   item.Duration,
			Genre:      item.Album.Genre.Name,
		}
		dt.AudioQuality.SamplingRate = item.Album.MaxSamplingRate
		dt.AudioQuality.BitDepth = item.Album.MaxBitDepth
		dt.AudioQuality.IsHiRes = item.Album.IsHiRes
		converted = append(converted, dt)
	}
	return converted, nil
}

func (c *Client) searchDab(query string) []DabTrack {
	searchURL := fmt.Sprintf("%s/search?q=%s&type=track", DABAPIBase, url.QueryEscape(query))
	req, _ := http.NewRequest("GET", searchURL, nil)
	
	resp, err := c.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	defer resp.Body.Close()

	var result struct {
		Tracks []DabTrack `json:"tracks"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Tracks
}

func (c *Client) ValidateToken() (string, error) {
	url := fmt.Sprintf("%s/auth/me", DABAPIBase)
	req, _ := http.NewRequest("GET", url, nil)
	
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		User *struct {
			ID interface{} `json:"id"`
		} `json:"user"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.User == nil {
		return "", fmt.Errorf("invalid session")
	}

	return fmt.Sprintf("%v", result.User.ID), nil
}

type QobuzSearchResponse struct {
	Tracks struct {
		Items []struct {
			ID         int    `json:"id"`
			Title      string `json:"title"`
			Duration   int    `json:"duration"`
			Album      struct {
				ID          interface{} `json:"id"`
				Title       string      `json:"title"`
				Image       struct { Large string `json:"large"` } `json:"image"`
				Genre       struct { Name string `json:"name"` } `json:"genre"`
				Artist      struct { ID int; Name string } `json:"artist"`
				MaxSamplingRate float64 `json:"maximum_sampling_rate"`
				MaxBitDepth     int     `json:"maximum_bit_depth"`
				IsHiRes         bool    `json:"hires"`
			} `json:"album"`
		} `json:"items"`
	} `json:"tracks"`
}
