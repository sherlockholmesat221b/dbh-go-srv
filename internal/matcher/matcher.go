package matcher

import (
	"database/sql"
	"fmt"
	"strings"
    "log"

	"dbh-go-srv/internal/dab"
	"dbh-go-srv/internal/database"
	"dbh-go-srv/internal/models"

	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
)

func MatchTrack(db *sql.DB, client *dab.Client, t models.Track, mode string, debugMode bool) *models.MatchResult {
	// 1. Check SQLite Registry first
	if db != nil {
		cachedID, err := database.GetDabIDFromSource(db, t.Type, t.SourceID)
		if err == nil && cachedID != "" {
			return &models.MatchResult{
				Track:       t,
				MatchStatus: "FOUND",
				DabTrackID:  &cachedID,
			}
		}
	}

	// 2. Metadata Enrichment: If YouTube, try to get ISRC from MusicBrainz
	if t.Type == "youtube" && t.ISRC == "" {
		if mbISRC := GetISRCFromMetadata(t.Artist, t.Title); mbISRC != "" {
			t.ISRC = mbISRC
			// Double check registry again with ISRC
			if cachedID, err := database.GetDabIDFromSource(db, "isrc", mbISRC); err == nil && cachedID != "" {
				return &models.MatchResult{
					Track:       t,
					MatchStatus: "FOUND",
					DabTrackID:  &cachedID,
				}
			}
		}
	}

	// 3. Search (Qobuz w/ DAB Fallback)
	useISRC := isValidISRC(t.ISRC)

	query := strings.TrimSpace(t.Artist + " " + t.Title)
	queryType := "text"

	if useISRC {
		query = t.ISRC
		queryType = "isrc"
	}

	if debugMode {
		log.Printf(
			"[MATCH] source=%s sourceID=%s title=%q artist=%q isrc=%q validISRC=%v queryType=%s query=%q",
			t.Type,
			t.SourceID,
			t.Title,
			t.Artist,
			t.ISRC,
			useISRC,
			queryType,
			query,
	)
	}

	results := client.Search(query)

	if debugMode {
		log.Printf("[MATCH] search results=%d for queryType=%s query=%q", len(results), queryType, query)
	}

	if len(results) == 0 {
		return &models.MatchResult{Track: t, MatchStatus: "NOT_FOUND"}
	}

	// 4. Fuzzy Matching Logic
	var bestMatch *dab.DabTrack
	var highestScore float64

	target := strings.ToLower(t.Artist + " " + t.Title)

	for _, cand := range results {
		candStr := strings.ToLower(cand.Artist + " " + cand.Title)
		score := strutil.Similarity(target, candStr, metrics.NewJaroWinkler())
		
		threshold := 0.85
		if mode == "strict" { threshold = 0.95 }

		// If we have an ISRC match, we treat it as a perfect 1.0
		if useISRC {
        	score = 1.0
        }

		if score >= threshold && score > highestScore {
			highestScore = score
			copyCand := cand
			bestMatch = &copyCand
		}
	}

	if bestMatch != nil {
	idStr := fmt.Sprintf("%d", bestMatch.ID)
		// 5. Update Registry Async for future speed
		go database.UpsertMapping(db, database.TrackMapping{
    		DabID:     idStr,
    		ISRC:      t.ISRC,
	    	SpotifyID: iif(t.Type == "spotify", t.SourceID, ""),
	     	YoutubeID: iif(t.Type == "youtube", t.SourceID, ""),
    	})

    	return &models.MatchResult{
	    	Track:       t,
	    	MatchStatus: "FOUND",
	    	DabTrackID:  &idStr,
	    	RawTrack:    bestMatch,
	    	Confidence:  highestScore,
    	}
    }

	return &models.MatchResult{Track: t, MatchStatus: "NOT_FOUND"}
}

func iif(condition bool, a, b string) string {
	if condition { return a }
	return b
}

func isValidISRC(s string) bool {
	if len(s) != 12 {
		return false
	}
	for i, r := range s {
		switch {
		case i < 2 && (r < 'A' || r > 'Z'):
			return false
		case i >= 2 && i < 5 && (r < 'A' || r > 'Z'):
			return false
		case i >= 5 && (r < '0' || r > '9'):
			return false
		}
	}
	return true
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
