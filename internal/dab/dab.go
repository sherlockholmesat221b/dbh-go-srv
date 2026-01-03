package dab

import (
	"context"
	"net/http"
	"sync"
	"time"

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
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.AddCookie(&http.Cookie{Name: "session", Value: c.Token})
	return c.HTTPClient.Do(req)
}

