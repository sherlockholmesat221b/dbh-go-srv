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

func (c *Client) ValidateToken() (string, error) {
	url := fmt.Sprintf("%s/auth/me", DABAPIBase)
	req, _ := http.NewRequest("GET", url, nil)
	
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// The API returns 200 even for invalid sessions, so we check the body
	var result struct {
		User *struct { // Use a pointer here to check for null
			ID interface{} `json:"id"`
		} `json:"user"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse auth response")
	}

	// CRITICAL FIX: Check if user object exists
	if result.User == nil {
		return "", fmt.Errorf("invalid or expired DAB session")
	}

	return fmt.Sprintf("%v", result.User.ID), nil
}
