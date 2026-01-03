package dab

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// DabTrack represents the DAB API response item
type DabTrack struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Artist       string `json:"artist"`
	AudioQuality struct {
		SamplingRate int `json:"maximumSampleRate"`
		BitDepth     int `json:"maximumBitDepth"`
	} `json:"audioQuality"`
}

func (c *Client) Search(query string) []DabTrack {
	searchURL := fmt.Sprintf("%s/search?q=%s&type=track", DABAPIBase, url.QueryEscape(query))
	
	req, _ := http.NewRequest("GET", searchURL, nil)
	resp, err := c.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result struct {
		Tracks []DabTrack `json:"tracks"`
	}
	
	// Handle both raw list and wrapped object
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}
	return result.Tracks
}
