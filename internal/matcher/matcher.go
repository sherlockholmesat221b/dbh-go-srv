package matcher

import (
	"database/sql"
	"fmt"
	"log"
	"strings"

	"dbh-go-srv/internal/dab"
	"dbh-go-srv/internal/database"
	"dbh-go-srv/internal/models"

	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"
)

// MatchTrack checks the SQLite registry first, then tries ISRC, and finally falls back to fuzzy matching.
func MatchTrack(db *sql.DB, client *dab.Client, t models.Track, mode string) *models.MatchResult {
	// --- STEP 0: Check SQLite Registry (Fail-Safe) ---
	if db != nil {
		cachedID, err := database.GetDabIDFromSource(db, t.Type, t.SourceID)
		if err == nil && cachedID != "" {
			t.Confidence = 1.0 // Cached matches are considered verified
			return &models.MatchResult{
				Track:       t,
				MatchStatus: "FOUND",
				DabTrackID:  &cachedID,
			}
		}
		// If there's a DB error (like a lock), we don't crash; we just log it and move to Step 1.
		if err != nil && err != sql.ErrNoRows {
			log.Printf("Database lookup skipped for %s: %v", t.Title, err)
		}
	}

	// --- STEP 1: Try ISRC Match ---
	if t.ISRC != "" {
		res := client.Search(t.ISRC)
		if len(res) > 0 {
			best := findBestQuality(res)
			idStr := fmt.Sprintf("%d", best.ID)
			t.Confidence = 1.0

			// Save to Registry asynchronously (ignoring error to prevent blocking)
			_ = database.UpsertMapping(db, database.TrackMapping{
				DabID:     idStr,
				ISRC:      t.ISRC,
				SpotifyID: iif(t.Type == "spotify", t.SourceID, ""),
				YoutubeID: iif(t.Type == "youtube", t.SourceID, ""),
			})

			return &models.MatchResult{
				Track:       t,
				MatchStatus: "FOUND",
				DabTrackID:  &idStr,
				RawTrack:    &best,
			}
		}
	}

	// --- STEP 2 & 3: Fuzzy Search Logic ---
	cleanTitle := t.Title
	if idx := strings.IndexAny(cleanTitle, "(["); idx != -1 {
		cleanTitle = strings.TrimSpace(cleanTitle[:idx])
	}

	query := strings.ToLower(t.Artist + " " + cleanTitle)
	results := client.Search(query)

	if len(results) == 0 && cleanTitle != t.Title {
		query = strings.ToLower(t.Artist + " " + t.Title)
		results = client.Search(query)
	}

	if len(results) == 0 {
		return &models.MatchResult{Track: t, MatchStatus: "NOT_FOUND"}
	}

	// --- STEP 4: Fuzzy Matching Loop ---
	var bestTrack *dab.DabTrack
	var highestScore float64

	for _, cand := range results {
		currentCand := cand
		candStr := strings.ToLower(currentCand.Artist + " " + currentCand.Title)
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

	// --- STEP 5: Return & Cache Result ---
	if bestTrack != nil {
		idStr := fmt.Sprintf("%d", bestTrack.ID)
		t.Confidence = highestScore

		// Store this fuzzy match in the DB so it's "Instant" for the next user
		_ = database.UpsertMapping(db, database.TrackMapping{
			DabID:     idStr,
			ISRC:      t.ISRC,
			SpotifyID: iif(t.Type == "spotify", t.SourceID, ""),
			YoutubeID: iif(t.Type == "youtube", t.SourceID, ""),
		})

		return &models.MatchResult{
			Track:       t,
			MatchStatus: "FOUND",
			DabTrackID:  &idStr,
			RawTrack:    bestTrack,
		}
	}

	return &models.MatchResult{Track: t, MatchStatus: "NOT_FOUND"}
}

// Helper to handle the mapping logic
func iif(condition bool, a, b string) string {
	if condition {
		return a
	}
	return b
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
