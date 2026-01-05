package matcher

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"
	"context"
)

var mbLimiter = rate.NewLimiter(rate.Every(time.Second), 1) // 1 req/s per MB guidelines

// MusicBrainzResponse simplified for ISRC extraction
type MusicBrainzResponse struct {
	Recordings []struct {
		ID    string   `json:"id"`
		ISRCs []string `json:"isrcs"`
		Score int      `json:"score"`
	} `json:"recordings"`
}

func GetISRCFromMetadata(artist, title string) string {
	_ = mbLimiter.Wait(context.Background())

	// Clean query for Lucene syntax
	query := fmt.Sprintf("artist:\"%s\" AND recording:\"%s\"", artist, title)
	searchURL := fmt.Sprintf("https://musicbrainz.org/ws/2/recording?query=%s&fmt=json", url.QueryEscape(query))

	req, _ := http.NewRequest("GET", searchURL, nil)
	// MusicBrainz requires a descriptive User-Agent
	req.Header.Set("User-Agent", "DBH-GO-SRV-Matcher/1.0 (https://github.com/sherlockholmesat221b/dbh-go-srv; sherlockholmesat221b@proton.me)")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return ""
	}
	defer resp.Body.Close()

	var res MusicBrainzResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return ""
	}

	// Return first ISRC found in top results
	for _, rec := range res.Recordings {
		if rec.Score > 80 && len(rec.ISRCs) > 0 {
			return rec.ISRCs[0]
		}
	}
	return ""
}
