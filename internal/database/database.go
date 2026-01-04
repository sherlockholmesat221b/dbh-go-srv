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

// InitDatabase runs the embedded schema and sets PRAGMAs for performance/stability
func InitDatabase(db *sql.DB) error {
	// WAL mode allows concurrent reads/writes without locking the UX
	_, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;")
	if err != nil {
		return err
	}
	_, err = db.Exec(schema)
	return err
}

func UpsertMapping(db *sql.DB, m TrackMapping) error {
	if db == nil { return nil } // Fail-safe
	query := `
	INSERT INTO track_registry (dab_id, isrc, spotify_id, youtube_id, last_updated)
	VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(dab_id) DO UPDATE SET
		isrc = COALESCE(NULLIF(isrc, ''), excluded.isrc),
		spotify_id = COALESCE(NULLIF(spotify_id, ''), excluded.spotify_id),
		youtube_id = COALESCE(NULLIF(youtube_id, ''), excluded.youtube_id),
		last_updated = CURRENT_TIMESTAMP;`

	_, err := db.Exec(query, m.DabID, m.ISRC, m.SpotifyID, m.YoutubeID)
	return err
}

func GetDabIDFromSource(db *sql.DB, sourceType, sourceID string) (string, error) {
	if db == nil { return "", fmt.Errorf("no db") }
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
		return "", fmt.Errorf("unsupported source type")
	}

	err := db.QueryRow(query, sourceID).Scan(&dabID)
	return dabID, err
}
