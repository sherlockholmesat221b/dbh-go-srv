package dab

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"dbh-go-srv/internal/models"
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

func (c *Client) AddTrackToLibrary(libraryID string, match models.MatchResult) error {
	if match.DabTrackID == nil {
		return fmt.Errorf("cannot add track: DabTrackID is nil")
	}

	payload := map[string]interface{}{
		"track": map[string]interface{}{
			"id":     *match.DabTrackID,
			"title":  match.Title,
			"artist": match.Artist,
			"audioQuality": map[string]interface{}{
				"maximumBitDepth":    24,
				"maximumSamplingRate": 96,
				"isHiRes":            true,
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
		return fmt.Errorf("failed to add track (Status %d): %s", resp.StatusCode, string(respBody))
	}
	
	return nil
}
