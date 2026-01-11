package parser

import (
	"database/sql"
	"encoding/csv"
	"errors"
	"net/http"
	"strings"

	"dbh-go-srv/internal/dab"
	"dbh-go-srv/internal/matcher" // Imported the matcher package
	"dbh-go-srv/internal/models"
)

// canonical header mapping
var headerAliases = map[string]string{
	"title":             "title",
	"track":             "title",
	"track_title":       "title",
	"name":              "title",
	"artist":            "artist",
	"artist_name":       "artist",
	"performer":         "artist",
	"album":             "album",
	"album_title":       "album",
	"isrc":              "isrc",
	"spotify":           "spotify",
	"spotify_uri":       "spotify",
	"spotify_track_uri": "spotify",
	"uri":               "spotify",
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// ParseCSV now takes DB, Client, and Match Mode to process tracks through the matcher
type ProgressCallback func(index, total int, res *models.MatchResult)
func ParseCSV(r *http.Request, db *sql.DB, client *dab.Client, mode string, onProgress ProgressCallback) ([]models.MatchResult, string, error) {
	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	// Read all records first to get the total count for the progress bar
	reader := csv.NewReader(file)
	allRecords, err := reader.ReadAll()
	if err != nil {
		return nil, "", err
	}

	if len(allRecords) < 2 {
		return nil, "", errors.New("CSV is empty or missing headers")
	}

	rawHeaders := allRecords[0]
	rows := allRecords[1:]
	columnMap := make(map[int]string)

	for i, h := range rawHeaders {
		if canonical, ok := headerAliases[normalize(h)]; ok {
			columnMap[i] = canonical
		}
	}

	if len(columnMap) == 0 {
		return nil, "", errors.New("CSV has no recognizable columns")
	}

	var results []models.MatchResult
	totalRows := len(rows)

	for i, record := range rows {
		var t models.Track
		for colIdx, v := range record {
			field, ok := columnMap[colIdx]
			if !ok { continue }
			val := strings.TrimSpace(v)
			if val == "" { continue }

			switch field {
			case "title": t.Title = val
			case "artist": t.Artist = val
			case "album": t.Album = val
			case "isrc": t.ISRC = val
			case "spotify":
				if strings.HasPrefix(val, "spotify:track:") {
					t.SourceID = val
					t.Type = "spotify"
				}
			}
		}

		if t.Title == "" && t.Artist == "" && t.SourceID == "" {
			continue
		}

		// Match and trigger SSE callback
		res := matcher.MatchTrack(db, client, t, mode)
		results = append(results, *res)

		if onProgress != nil {
			onProgress(i+1, totalRows, res)
		}
	}

	return results, header.Filename, nil
}