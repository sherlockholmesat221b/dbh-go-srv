package models

type Track struct {
	Title      string  `json:"title"`
	Artist     string  `json:"artist"`
	Album      string  `json:"album,omitempty"`
	ISRC       string  `json:"isrc,omitempty"`
	SourceID   string  `json:"source_id"`
	Confidence float64 `json:"confidence"`
}

type MatchResult struct {
	Track
	MatchStatus string  `json:"match_status"` // FOUND | NOT_FOUND
	DabTrackID  *string `json:"dab_track_id"`
}

type Report struct {
	Library     LibraryInfo   `json:"library"`
	Source      SourceInfo    `json:"source"`
	MatchingMode string       `json:"matching_mode"`
	Timestamp   string        `json:"timestamp"`
	Tracks      []MatchResult `json:"tracks"`
}

type LibraryInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SourceInfo struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}
