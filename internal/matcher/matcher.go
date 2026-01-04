package matcher

import (
	"fmt"
	"strings"
	"dbh-go-srv/internal/dab"
	"dbh-go-srv/internal/models"
	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
)

func MatchTrack(client *dab.Client, t models.Track, mode string) *models.MatchResult {
	// 1. Try ISRC Match first
	if t.ISRC != "" {
		res := client.Search(t.ISRC)
		if len(res) > 0 {
			best := findBestQuality(res)
			idStr := fmt.Sprintf("%d", best.ID)
			
			// If matched by ISRC, confidence is 100%
			t.Confidence = 1.0 

			return &models.MatchResult{
				Track:       t, 
				MatchStatus: "FOUND", 
				DabTrackID:  &idStr,
				RawTrack:    &best,
			}
		}
	}
    
	// 2. Prepare fuzzy search query
	cleanTitle := t.Title
	if idx := strings.IndexAny(cleanTitle, "(["); idx != -1 {
		cleanTitle = strings.TrimSpace(cleanTitle[:idx])
	}

	query := strings.ToLower(t.Artist + " " + cleanTitle)
	results := client.Search(query)

	// 3. Fallback to full title if clean title yields nothing
	if len(results) == 0 && cleanTitle != t.Title {
		query = strings.ToLower(t.Artist + " " + t.Title)
		results = client.Search(query)
	}

	if len(results) == 0 {
		return &models.MatchResult{Track: t, MatchStatus: "NOT_FOUND"}
	}
	
	// 4. Fuzzy Matching Loop
	var bestTrack *dab.DabTrack
	var highestScore float64

	for _, cand := range results {
		currentCand := cand
		candStr := strings.ToLower(currentCand.Artist + " " + currentCand.Title)
		
		// Calculate the score
		score := strutil.Similarity(query, candStr, metrics.NewJaroWinkler())
		
		threshold := 0.85
		if mode == "strict" {
			threshold = 0.95
		}

		if score > highestScore && score >= threshold {
			highestScore = score
			bestTrack = &currentCand
		}
	}

	// 5. Return result with Metadata and Confidence Score
	if bestTrack != nil {
		idStr := fmt.Sprintf("%d", bestTrack.ID)
		
		// Update the confidence with the actual similarity score
		t.Confidence = highestScore 

		return &models.MatchResult{
			Track:       t,
			MatchStatus: "FOUND",
			DabTrackID:  &idStr,
			RawTrack:    bestTrack, 
		}
	}

	return &models.MatchResult{Track: t, MatchStatus: "NOT_FOUND"}
}

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
