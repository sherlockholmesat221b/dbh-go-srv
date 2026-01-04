package dab

import (
	"fmt"
	"dbh-go-srv/internal/models"
)

// GetLibraryTracks fetches all tracks currently in a DAB library.
// This is essential for the Force Sync "Pruning" logic.
func (c *Client) GetLibraryTracks(libraryID string) ([]DabTrack, error) {
	var result struct {
		Tracks []DabTrack `json:"tracks"`
	}

	// GET /libraries/{id}/tracks
	err := c.DoRequest("GET", fmt.Sprintf("/libraries/%s/tracks", libraryID), nil, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch library tracks: %w", err)
	}

	return result.Tracks, nil
}

// CreateLibrary creates a new library on DAB
func (c *Client) CreateLibrary(name string) (string, error) {
	payload := map[string]interface{}{
		"name":        name,
		"description": "Created via DABHounds Go API",
		"isPublic":    true,
	}

	var result struct {
		Library struct {
			ID string `json:"id"`
		} `json:"library"`
	}

	err := c.DoRequest("POST", "/libraries", payload, &result)
	if err != nil {
		return "", err
	}

	return result.Library.ID, nil
}

// AddTrackToLibrary - Preserves your EXACT schema for full track metadata
func (c *Client) AddTrackToLibrary(libraryID string, track DabTrack) error {
	payload := map[string]interface{}{
		"track": map[string]interface{}{
			"id":          fmt.Sprintf("%d", track.ID),
			"title":       track.Title,
			"artist":      track.Artist,
			"artistId":    track.ArtistID,
			"albumTitle":  track.AlbumTitle,
			"albumCover":  track.AlbumCover,
			"albumId":     fmt.Sprintf("%v", track.AlbumID), 
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

	return c.DoRequest("POST", fmt.Sprintf("/libraries/%s/tracks", libraryID), payload, nil)
}

// AddTrackByID - Specifically for SQLite cache hits where we only have the ID
func (c *Client) AddTrackByID(libraryID string, trackID string) error {
	payload := map[string]interface{}{
		"trackId": trackID,
	}
	return c.DoRequest("POST", fmt.Sprintf("/libraries/%s/tracks", libraryID), payload, nil)
}

// UpdateLibrary - Used for Syncing metadata (PATCH)
func (c *Client) UpdateLibrary(id string, name string, desc string) error {
	payload := map[string]interface{}{
		"name":        name,
		"description": desc,
	}
	return c.DoRequest("PATCH", fmt.Sprintf("/libraries/%s", id), payload, nil)
}

// RemoveTrackFromLibrary - Used for Pruning tracks in Force Sync (DELETE)
func (c *Client) RemoveTrackFromLibrary(libID string, trackID string) error {
	return c.DoRequest("DELETE", fmt.Sprintf("/libraries/%s/tracks/%s", libID, trackID), nil, nil)
}

// GetLibraryInfo - Fetches current state to compare titles/descriptions
func (c *Client) GetLibraryInfo(id string) (*models.LibraryInfo, error) {
	var result struct {
		Library models.LibraryInfo `json:"library"`
	}
	err := c.DoRequest("GET", fmt.Sprintf("/libraries/%s", id), nil, &result)
	if err != nil {
		return nil, err
	}
	return &result.Library, err
}
