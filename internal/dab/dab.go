package dab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	UserAgent  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/1337.0.0.0 Safari/537.36"
	DABAPIBase = "https://dabmusic.xyz/api"
)

type Client struct {
	HTTPClient *http.Client
	Limiter    *rate.Limiter
	Token      string
}

var (
	instance *Client
	once     sync.Once
)

func GetClient(token string) *Client {
	once.Do(func() {
		instance = &Client{
			HTTPClient: &http.Client{Timeout: 30 * time.Second},
			// 15 requests per 10 seconds = 1.5 req/sec
			Limiter: rate.NewLimiter(rate.Every(666*time.Millisecond), 1),
		}
	})
	instance.Token = token
	return instance
}

// Do handles the low-level HTTP headers and rate limiting
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	c.Limiter.Wait(context.Background())

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	
	// Manual Cookie header as requested
	req.Header.Set("Cookie", fmt.Sprintf("session=%s", c.Token))

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json") // Added for PATCH/POST support
	req.Header.Set("Referer", "https://dabmusic.xyz/")
	req.Header.Set("Origin", "https://dabmusic.xyz")

	return c.HTTPClient.Do(req)
}

// DoRequest is a high-level helper to handle JSON body encoding/decoding
func (c *Client) DoRequest(method, path string, body interface{}, result interface{}) error {
	url := DABAPIBase + path
	var bodyReader io.Reader

	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewBuffer(jsonBytes)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return err
	}

	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("DAB API error: status %d", resp.StatusCode)
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}

	return nil
}
