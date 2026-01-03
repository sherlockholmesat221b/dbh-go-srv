package dab

import (
	"context"
	"net/http"
	"sync"
	"time"
    "fmt"

	"golang.org/x/time/rate"
)

const (
	UserAgent   = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/1337.0.0.0 Safari/537.36"
	DABAPIBase  = "https://dabmusic.xyz/api"
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

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	c.Limiter.Wait(context.Background())

	// 1. Set the exact UA again
	req.Header.Set("User-Agent", UserAgent)

	// 2. Auth: Some APIs are picky about "Bearer " vs "bearer " or just the token.
    // Based on your curl, "Bearer " is correct.
	req.Header.Set("Authorization", "Bearer "+c.Token)
    
    // 3. IMPORTANT: Explicitly set the Cookie header string 
    // Sometimes req.AddCookie acts differently than a manual header set in Go
    req.Header.Set("Cookie", fmt.Sprintf("session=%s", c.Token))

	// 4. Mimic Browser more closely
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://dabmusic.xyz/")
	req.Header.Set("Origin", "https://dabmusic.xyz")

	return c.HTTPClient.Do(req)
}
