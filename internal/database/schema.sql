-- The Master Registry: Links platform IDs to a single DAB ID (Qobuz ID)
CREATE TABLE IF NOT EXISTS track_registry (
    dab_id TEXT PRIMARY KEY,
    isrc TEXT,
    spotify_id TEXT,
    youtube_id TEXT,
    last_updated DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indices for fast lookups during the matching phase
CREATE INDEX IF NOT EXISTS idx_isrc ON track_registry(isrc) WHERE isrc IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_spotify ON track_registry(spotify_id) WHERE spotify_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_youtube ON track_registry(youtube_id) WHERE youtube_id IS NOT NULL;
