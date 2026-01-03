package matcher

import (
	"strings"
	"dbh-go-srv/internal/dab"
	"dbh-go-srv/internal/models"
	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
)

func MatchTrack(client *dab.Client, t models.Track, mode string) *models.MatchResult {
	if t.ISRC != "" {
		res := client.Search(t.ISRC)
		if len(res) > 0 {
			best := findBestQuality(res)
			return &models.MatchResult{Track: t, MatchStatus: "FOUND", DabTrackID: &best.ID}
		}
	}

	if mode == "strict" {
		return &models.MatchResult{Track: t, MatchStatus: "NOT_FOUND"}
	}

	query := strings.ToLower(t.Artist + " " + t.Title)
	results := client.Search(query)
	
	var bestID string
	var highestScore float64

	for _, cand := range results {
		candStr := strings.ToLower(cand.Artist + " " + cand.Title)
		// Using Jaro-Winkler as a robust Go alternative to fuzzywuzzy
		score := strutil.Similarity(query, candStr, metrics.NewJaroWinkler())
		
		if score > highestScore && score >= 0.85 { // 0.85 similarity threshold
			highestScore = score
			bestID = cand.ID
		}
	}

	if bestID != "" {
		return &models.MatchResult{Track: t, MatchStatus: "FOUND", DabTrackID: &bestID}
	}

	return &models.MatchResult{Track: t, MatchStatus: "NOT_FOUND"}
}

// findBestQuality replicates Python sorted(tracks, key=get_quality, reverse=True)[0]
func findBestQuality(tracks []dab.DabTrack) dab.DabTrack {
	best := tracks[0]
	for _, t := range tracks {
		if t.AudioQuality.SamplingRate > best.AudioQuality.SamplingRate {
			best = t
		} else if t.AudioQuality.SamplingRate == best.AudioQuality.SamplingRate {
			if t.AudioQuality.BitDepth > best.AudioQuality.BitDepth {
				best = t
			}
		}
	}
	return best
}
