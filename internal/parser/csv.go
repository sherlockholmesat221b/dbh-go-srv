package parser

import (
	"encoding/csv"
//	"fmt"
	"io"
	"net/http"
	"dbh-go-srv/internal/models"
)

// ParseCSV handles multipart file uploads from the Web API
func ParseCSV(r *http.Request) ([]models.Track, string, error) {
	file, header, err := r.FormFile("file")
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	// Skip header
	if _, err := reader.Read(); err != nil {
		return nil, "", err
	}

	var tracks []models.Track
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", err
		}

		// Expected order: title, artist, album, isrc
		if len(record) < 2 {
			continue
		}

		t := models.Track{
			Title:  record[0],
			Artist: record[1],
		}
		if len(record) > 2 { t.Album = record[2] }
		if len(record) > 3 { t.ISRC = record[3] }

		tracks = append(tracks, t)
	}

	return tracks, header.Filename, nil
}
