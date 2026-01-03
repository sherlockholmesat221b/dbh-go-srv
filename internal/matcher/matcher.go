package matcher

import (
	"fmt" // Added missing import
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
			idStr := fmt.Sprintf("%d", best.ID) // Convert int to string
			return &models.MatchResult{
				Track:       t, 
				MatchStatus: "FOUND", 
				DabTrackID:  &idStr,
			}
		}
	}
    
	// Clean title: Remove anything in brackets/parentheses for better search
	cleanTitle := t.Title
	if idx := strings.IndexAny(cleanTitle, "(["); idx != -1 {
		cleanTitle = strings.TrimSpace(cleanTitle[:idx])
	}

	query := strings.ToLower(t.Artist + " " + cleanTitle)
	results := client.Search(query)

	if mode == "strict" && len(results) == 0 {
		return &models.MatchResult{Track: t, MatchStatus: "NOT_FOUND"}
	}

	// If the clean search returned nothing, try one last time with the full title
	if len(results) == 0 && cleanTitle != t.Title {
		query = strings.ToLower(t.Artist + " " + t.Title)
		results = client.Search(query)
	}
	
	var bestID string
	var highestScore float64

	for _, cand := range results {
		candStr := strings.ToLower(cand.Artist + " " + cand.Title)
		score := strutil.Similarity(query, candStr, metrics.NewJaroWinkler())
		
		threshold := 0.85
		if mode == "strict" {
			threshold = 0.95
		}

		if score > highestScore && score >= threshold {
			highestScore = score
			bestID = fmt.Sprintf("%d", cand.ID) // Convert int to string
		}
	}

	if bestID != "" {
		// We need a local copy to get a stable pointer
		finalID := bestID 
		return &models.MatchResult{
			Track:       t, 
			MatchStatus: "FOUND", 
			DabTrackID:  &finalID,
		}
	}

	return &models.MatchResult{Track: t, MatchStatus: "NOT_FOUND"}
}

// findBestQuality replicates Python logic for picking the best audio quality
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
