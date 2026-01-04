-- The Master Registry: Links different platform IDs to a single DAB Anchor
CREATE TABLE IF NOT EXISTS track_registry (
    dab_id TEXT PRIMARY KEY,
    isrc TEXT,
    spotify_id TEXT,
    youtube_id TEXT,
    last_updated DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indices for O(log n) lookups
CREATE INDEX IF NOT EXISTS idx_isrc ON track_registry(isrc);
CREATE INDEX IF NOT EXISTS idx_spotify ON track_registry(spotify_id);
CREATE INDEX IF NOT EXISTS idx_youtube ON track_registry(youtube_id);

-- Track the original source of a Library
CREATE TABLE IF NOT EXISTS library_metadata (
    dab_library_id TEXT PRIMARY KEY,
    owner_id TEXT,
    source_url TEXT,
    source_type TEXT,
    last_sync DATETIME DEFAULT CURRENT_TIMESTAMP
);
