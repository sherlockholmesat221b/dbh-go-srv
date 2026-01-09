package parser

import (
	"encoding/csv"
	"errors"
	"io"
	"net/http"
	"strings"

	"dbh-go-srv/internal/models"
)

// canonical header mapping
var headerAliases = map[string]string{
	"title":               "title",
	"track":               "title",
	"track_title":         "title",
	"name":                "title",

	"artist":              "artist",
	"artist_name":         "artist",
	"performer":           "artist",

	"album":               "album",
	"album_title":         "album",

	"isrc":                "isrc",

	"spotify":             "spotify",
	"spotify_uri":         "spotify",
	"spotify_track_uri":   "spotify",
	"uri":                 "spotify",
}

func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// ParseCSV handles multipart file uploads from the Web API
func ParseCSV(r *http.Request) ([]models.Track, string, error) {
	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// ---- Read header row ----
	rawHeaders, err := reader.Read()
	if err != nil {
		return nil, "", err
	}

	columnMap := make(map[int]string)

	for i, h := range rawHeaders {
		if canonical, ok := headerAliases[normalize(h)]; ok {
			columnMap[i] = canonical
		}
	}

	if len(columnMap) == 0 {
		return nil, "", errors.New("CSV has no recognizable columns")
	}

	var tracks []models.Track

	// ---- Read rows ----
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", err
		}

		var t models.Track

		for i, v := range record {
			field, ok := columnMap[i]
			if !ok {
				continue
			}

			val := strings.TrimSpace(v)
			if val == "" {
				continue
			}

			switch field {
			case "title":
				t.Title = val
			case "artist":
				t.Artist = val
			case "album":
				t.Album = val
			case "isrc":
				t.ISRC = val
			case "spotify":
				if strings.HasPrefix(val, "spotify:track:") {
					t.SourceID = val
					t.Type = "spotify"
				}
			}
		}

		// Skip totally empty rows
		if t.Title == "" && t.Artist == "" && t.SourceID == "" {
			continue
		}

		tracks = append(tracks, t)
	}

	return tracks, header.Filename, nil
}