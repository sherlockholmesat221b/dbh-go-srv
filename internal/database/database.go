package database

import (
	"database/sql"
	_ "embed"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schema string

type TrackMapping struct {
	DabID     string
	ISRC      string
	SpotifyID string
	YoutubeID string
}

// InitDatabase runs the embedded schema and sets performance PRAGMAs
func InitDatabase(db *sql.DB) error {
	// WAL mode is critical for SSE performance so writes don't block concurrent match lookups
	_, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA cache_size=-2000;")
	if err != nil {
		return err
	}
	_, err = db.Exec(schema)
	return err
}

// UpsertMapping inserts or updates the registry. 
// It uses COALESCE to ensure we don't wipe out existing IDs from other platforms.
func UpsertMapping(db *sql.DB, m TrackMapping) error {
	if db == nil { return nil }
	
	query := `
	INSERT INTO track_registry (dab_id, isrc, spotify_id, youtube_id, last_updated)
	VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(dab_id) DO UPDATE SET
		isrc = COALESCE(NULLIF(excluded.isrc, ''), track_registry.isrc),
		spotify_id = COALESCE(NULLIF(excluded.spotify_id, ''), track_registry.spotify_id),
		youtube_id = COALESCE(NULLIF(excluded.youtube_id, ''), track_registry.youtube_id),
		last_updated = CURRENT_TIMESTAMP;`

	_, err := db.Exec(query, m.DabID, m.ISRC, m.SpotifyID, m.YoutubeID)
	return err
}

// GetDabIDFromSource looks up a DAB ID based on platform-specific IDs
func GetDabIDFromSource(db *sql.DB, sourceType, sourceID string) (string, error) {
	if db == nil || sourceID == "" { return "", fmt.Errorf("invalid lookup") }
	
	var dabID string
	var query string

	switch sourceType {
	case "spotify":
		query = "SELECT dab_id FROM track_registry WHERE spotify_id = ?"
	case "youtube":
		query = "SELECT dab_id FROM track_registry WHERE youtube_id = ?"
	case "isrc":
		query = "SELECT dab_id FROM track_registry WHERE isrc = ?"
	default:
		return "", fmt.Errorf("unsupported source type: %s", sourceType)
	}

	err := db.QueryRow(query, sourceID).Scan(&dabID)
	return dabID, err
}
