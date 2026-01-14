package dab

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
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
	HTTPClient    *http.Client
	Limiter       *rate.Limiter
	Token         string
	QobuzID       string
	QobuzUserAuth string
	Debug         bool
}

func (c *Client) dbg(format string, args ...any) {
	if c.Debug {
		log.Printf("[DAB][DEBUG] "+format, args...)
	}
}

// GetClient initializes the client
func GetClient(token string, debugMode bool) *Client {
	qobuzID := os.Getenv("QOBUZ_APP_ID")
	if qobuzID == "" {
		log.Fatal("CRITICAL: QOBUZ_APP_ID must be set")
	}

	userAuth := os.Getenv("QOBUZ_USER_AUTH_TOKEN")
	if userAuth == "" {
		log.Fatal("CRITICAL: QOBUZ_USER_AUTH_TOKEN must be set")
	}

	c := &Client{
		HTTPClient:    &http.Client{Timeout: 30 * time.Second},
		Limiter:       rate.NewLimiter(rate.Every(666*time.Millisecond), 1),
		Token:         token,
		QobuzID:       qobuzID,
		QobuzUserAuth: userAuth,
		Debug:         debugMode,
	}

	c.dbg("Client initialized")
	c.dbg("QOBUZ_APP_ID=%s", qobuzID)
	c.dbg("QOBUZ_USER_AUTH_TOKEN=%s…%s", userAuth[:6], userAuth[len(userAuth)-4:])

	return c
}

// Do handles rate limiting and headers
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if err := c.Limiter.Wait(context.Background()); err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Cookie", fmt.Sprintf("session=%s", c.Token))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://dabmusic.xyz/")
	req.Header.Set("Origin", "https://dabmusic.xyz")

	c.dbg("HTTP %s %s", req.Method, req.URL.String())
	return c.HTTPClient.Do(req)
}

// Search: Qobuz first, DAB fallback
func (c *Client) Search(query string) []DabTrack {
	c.dbg("Search query=%q", query)

	qTracks, err := c.searchQobuz(query)
	if err == nil && len(qTracks) > 0 {
		c.dbg("Qobuz hit: %d tracks", len(qTracks))
		return qTracks
	}

	c.dbg("Qobuz miss, falling back to DAB")
	return c.searchDab(query)
}

func (c *Client) searchQobuz(query string) ([]DabTrack, error) {
	searchURL := fmt.Sprintf(
		"%s/track/search?query=%s&limit=5&app_id=%s&user_auth_token=%s",
		QobuzAPIBase,
		url.QueryEscape(query),
		c.QobuzID,
		url.QueryEscape(c.QobuzUserAuth),
	)

	c.dbg("Qobuz URL=%s", searchURL)

	req, _ := http.NewRequest("GET", searchURL, nil)
	req.Header.Set("X-App-Id", c.QobuzID)
	req.Header.Set("X-User-Auth-Token", c.QobuzUserAuth)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qobuz status %s", resp.Status)
	}

	var qRes QobuzSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&qRes); err != nil {
		return nil, err
	}

	var out []DabTrack
	for _, item := range qRes.Tracks.Items {
		var dt DabTrack
		dt.ID = item.ID
		dt.Title = item.Title
		dt.Artist = item.Album.Artist.Name
		dt.ArtistID = item.Album.Artist.ID
		dt.AlbumTitle = item.Album.Title
		dt.AlbumCover = item.Album.Image.Large
		dt.AlbumID = item.Album.ID
		dt.Duration = item.Duration
		dt.Genre = item.Album.Genre.Name
		dt.AudioQuality.SamplingRate = item.Album.MaxSamplingRate
		dt.AudioQuality.BitDepth = item.Album.MaxBitDepth
		dt.AudioQuality.IsHiRes = item.Album.IsHiRes
		out = append(out, dt)
	}

	return out, nil
}

func (c *Client) searchDab(query string) []DabTrack {
	searchURL := fmt.Sprintf("%s/search?q=%s&type=track", DABAPIBase, url.QueryEscape(query))
	c.dbg("DAB URL=%s", searchURL)

	req, _ := http.NewRequest("GET", searchURL, nil)
	resp, err := c.Do(req)
	if err != nil {
		c.dbg("DAB error: %v", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.dbg("DAB status=%s", resp.Status)
		return nil
	}

	var result struct {
		Tracks []DabTrack `json:"tracks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.dbg("DAB decode error: %v", err)
		return nil
	}

	return result.Tracks
}

func (c *Client) ValidateToken() (string, error) {
	req, _ := http.NewRequest("GET", DABAPIBase+"/auth/me", nil)
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

/* ---------- Qobuz ---------- */

type QobuzSearchResponse struct {
	Tracks struct {
		Items []struct {
			ID       int    `json:"id"`
			Title    string `json:"title"`
			Duration int    `json:"duration"`
			Album    struct {
				ID    interface{} `json:"id"`
				Title string      `json:"title"`
				Image struct {
					Large string `json:"large"`
				} `json:"image"`
				Genre struct {
					Name string `json:"name"`
				} `json:"genre"`
				Artist struct {
					ID   int    `json:"id"`
					Name string `json:"name"`
				} `json:"artist"`
				MaxSamplingRate float64 `json:"maximum_sampling_rate"`
				MaxBitDepth     int     `json:"maximum_bit_depth"`
				IsHiRes         bool    `json:"hires"`
			} `json:"album"`
		} `json:"items"`
	} `json:"tracks"`
}