package dab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func (c *Client) CreateLibrary(name string) (string, error) {
	payload := map[string]interface{}{
		"name":        name,
		"description": "Created via DABHounds Go API",
		"isPublic":    true,
	}

	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/libraries", DABAPIBase)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("DAB API %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Library struct {
			ID string `json:"id"`
		} `json:"library"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode library response: %w", err)
	}

	return result.Library.ID, nil
}

func (c *Client) AddTrackToLibrary(libraryID string, track DabTrack) error {
	// Replicating the EXACT AddTrackToLibraryRequest schema
	payload := map[string]interface{}{
		"track": map[string]interface{}{
			"id":          fmt.Sprintf("%d", track.ID),
			"title":       track.Title,
			"artist":      track.Artist,
			"artistId":    track.ArtistID,
			"albumTitle":  track.AlbumTitle,
			"albumCover":  track.AlbumCover,
			"albumId":     fmt.Sprintf("%v", track.AlbumID), // Handles alphanumeric
			"releaseDate": track.ReleaseDate,
			"genre":       track.Genre,
			"duration":    track.Duration,
			"audioQuality": map[string]interface{}{
				"maximumBitDepth":     track.AudioQuality.BitDepth,
				"maximumSamplingRate": track.AudioQuality.SamplingRate,
				"isHiRes":             track.AudioQuality.IsHiRes,
			},
		},
	}

	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/libraries/%s/tracks", DABAPIBase, libraryID)
	
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DAB Add Error (%d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}
