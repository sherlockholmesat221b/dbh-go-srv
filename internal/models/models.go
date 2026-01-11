package models

type Track struct {
	Title      string  `json:"title"`
	Artist     string  `json:"artist"`
	Album      string  `json:"album,omitempty"`
	ISRC       string  `json:"isrc,omitempty"`
	SourceID   string  `json:"source_id"`
	Type       string  `json:"type"` // "spotify" or "youtube"
}

type MatchResult struct {
	Track
	MatchStatus string      `json:"match_status"`
	DabTrackID  *string     `json:"dab_track_id"`
	RawTrack    interface{} `json:"raw_track"` // Exported for JSON streaming
    Confidence float64 `json:"confidence"`
}
