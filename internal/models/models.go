package models

type Track struct {
	Title      string  `json:"title"`
	Artist     string  `json:"artist"`
	Album      string  `json:"album,omitempty"`
	ISRC       string  `json:"isrc,omitempty"`
	SourceID   string  `json:"source_id"`
    Type       string  `json:"type"`
	Confidence float64 `json:"confidence"`
}

type MatchResult struct {
	Track
	MatchStatus string      `json:"match_status"`
	DabTrackID  *string     `json:"dab_track_id"`
	RawTrack    interface{} `json:"-"` // Use interface{} to avoid circular import
}

type Report struct {
    UserID       string        `json:"user_id"`
	Library      LibraryInfo   `json:"library"`
	Source       SourceInfo    `json:"source"`
	MatchingMode string        `json:"matching_mode"`
	Timestamp    string        `json:"timestamp"`
	Tracks       []MatchResult `json:"tracks"`
}

type LibraryInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SourceInfo struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}
