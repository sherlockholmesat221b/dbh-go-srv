package dab

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
    "io"
)

// DabTrack represents the DAB API response item
type DabTrack struct {
	ID           int     `json:"id"`
	Title        string  `json:"title"`
	Artist       string  `json:"artist"`
	ArtistID     int     `json:"artistId"`
	AlbumTitle   string  `json:"albumTitle"`
	AlbumCover   string  `json:"albumCover"`
	AlbumID      any     `json:"albumId"` // Handles both number or string (alphanumeric)
	ReleaseDate  string  `json:"releaseDate"`
	Genre        string  `json:"genre"`
	Duration     int     `json:"duration"`
	AudioQuality struct {
		SamplingRate float64 `json:"maximumSampleRate"`
		BitDepth     int     `json:"maximumBitDepth"`
		IsHiRes      bool    `json:"isHiRes"`
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

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var result struct {
		Tracks []DabTrack `json:"tracks"`
	}
	
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Printf("DECODE ERROR: %v\n", err)
		return nil
	}
	return result.Tracks
}
