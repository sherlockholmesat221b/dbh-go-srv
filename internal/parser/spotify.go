package parser

import (
	"context"
	"fmt"
	"log"
	"strings"

	"dbh-go-srv/internal/models"
	"dbh-go-srv/internal/spotifetch"
	"github.com/zmb3/spotify/v2"
)

type SpotifyParser struct {
	client    *spotify.Client
	debugMode bool
}

func NewSpotifyParser(client *spotify.Client, debugMode bool) *SpotifyParser {
	return &SpotifyParser{
		client:    client,
		debugMode: debugMode,
	}
}

// Parse tries SpotiFLAC first, then falls back to official Spotify API
func (p *SpotifyParser) Parse(ctx context.Context, url string) ([]models.Track, string, error) {
	if p.debugMode {
		log.Printf("[SPOTIFY] Parse start url=%q", url)
	}

	// --- Step 1: Try SpotiFLAC metadata fetch ---
	meta, err := spotifetch.GetFilteredSpotifyData(ctx, url, false, 0)
	if err == nil {
		tracks, name := convertMetadataToTracks(meta)

		if p.debugMode {
			log.Printf(
				"[SPOTIFY] spotifetch SUCCESS name=%q tracks=%d metaType=%T",
				name,
				len(tracks),
				meta,
			)
			for i, t := range tracks {
				log.Printf(
					"[SPOTIFY] spotifetch track[%d] title=%q artist=%q album=%q isrc=%q sourceID=%q",
					i,
					t.Title,
					t.Artist,
					t.Album,
					t.ISRC,
					t.SourceID,
				)
			}
		}

		return tracks, name, nil
	}

	if p.debugMode {
		log.Printf("[SPOTIFY] spotifetch FAILED: %v — falling back to Web API", err)
	}

	// --- Step 2: Fallback to official Spotify Web API ---
	id, mediaType, err := p.parseURL(url)
	if err != nil {
		return nil, "", fmt.Errorf("spotify parse url: %w", err)
	}

	if p.debugMode {
		log.Printf("[SPOTIFY] fallback mediaType=%s id=%s", mediaType, id)
	}

	switch mediaType {
	case "playlist":
		return p.handlePlaylist(ctx, id)
	case "album":
		return p.handleAlbum(ctx, id)
	case "track":
		return p.handleTrack(ctx, id)
	default:
		return nil, "", fmt.Errorf("unsupported spotify type: %s", mediaType)
	}
}

// --- Conversion from SpotiFLAC metadata to models.Track ---
func convertMetadataToTracks(meta interface{}) ([]models.Track, string) {
	tracks := []models.Track{}

	switch v := meta.(type) {
	case spotifetch.TrackResponse:
		t := v.Track
		tracks = append(tracks, models.Track{
			Title:    t.Name,
			Artist:   t.Artists,
			Album:    t.AlbumName,
			ISRC:     t.ISRC,
			SourceID: t.SpotifyID,
			Type:     "spotify",
		})
		return tracks, t.Name

	case *spotifetch.AlbumResponsePayload:
		for _, t := range v.TrackList {
			tracks = append(tracks, models.Track{
				Title:    t.Name,
				Artist:   t.Artists,
				Album:    v.AlbumInfo.Name,
				ISRC:     t.ISRC,
				SourceID: t.SpotifyID,
				Type:     "spotify",
			})
		}
		return tracks, v.AlbumInfo.Name

	case spotifetch.PlaylistResponsePayload:
		for _, t := range v.TrackList {
			tracks = append(tracks, models.Track{
				Title:    t.Name,
				Artist:   t.Artists,
				Album:    t.AlbumName,
				ISRC:     t.ISRC,
				SourceID: t.SpotifyID,
				Type:     "spotify",
			})
		}
		return tracks, v.PlaylistInfo.Owner.Name
	}

	return nil, ""
}

// --- Web API functions ---

func (p *SpotifyParser) handlePlaylist(ctx context.Context, id spotify.ID) ([]models.Track, string, error) {
	res, err := p.client.GetPlaylist(ctx, id)
	if err != nil {
		return nil, "", fmt.Errorf("get playlist: %w", err)
	}

	var tracks []models.Track
	trackPage := res.Tracks

	for {
		for _, item := range trackPage.Tracks {
			if item.Track.ID != "" && !item.IsLocal {
				t := p.transform(item.Track)
				tracks = append(tracks, t)

				if p.debugMode {
					log.Printf(
						"[SPOTIFY] playlist track title=%q artist=%q album=%q isrc=%q sourceID=%q",
						t.Title,
						t.Artist,
						t.Album,
						t.ISRC,
						t.SourceID,
					)
				}
			}
		}

		err = p.client.NextPage(ctx, &trackPage)
		if err == spotify.ErrNoMorePages {
			break
		}
		if err != nil {
			return tracks, res.Name, fmt.Errorf("playlist pagination error: %w", err)
		}
	}

	return tracks, res.Name, nil
}

func (p *SpotifyParser) handleAlbum(ctx context.Context, id spotify.ID) ([]models.Track, string, error) {
	res, err := p.client.GetAlbum(ctx, id)
	if err != nil {
		return nil, "", fmt.Errorf("get album: %w", err)
	}

	var ids []spotify.ID
	for _, t := range res.Tracks.Tracks {
		ids = append(ids, t.ID)
	}

	var tracks []models.Track
	for i := 0; i < len(ids); i += 50 {
		end := i + 50
		if end > len(ids) {
			end = len(ids)
		}

		fullTracks, err := p.client.GetTracks(ctx, ids[i:end])
		if err != nil {
			return nil, "", fmt.Errorf("get full tracks for album: %w", err)
		}

		for _, ft := range fullTracks {
			t := p.transform(*ft)
			tracks = append(tracks, t)

			if p.debugMode {
				log.Printf(
					"[SPOTIFY] album track title=%q artist=%q album=%q isrc=%q sourceID=%q",
					t.Title,
					t.Artist,
					t.Album,
					t.ISRC,
					t.SourceID,
				)
			}
		}
	}

	return tracks, res.Name, nil
}

func (p *SpotifyParser) handleTrack(ctx context.Context, id spotify.ID) ([]models.Track, string, error) {
	res, err := p.client.GetTrack(ctx, id)
	if err != nil {
		return nil, "", fmt.Errorf("get track: %w", err)
	}

	t := p.transform(*res)

	if p.debugMode {
		log.Printf(
			"[SPOTIFY] single track title=%q artist=%q album=%q isrc=%q sourceID=%q",
			t.Title,
			t.Artist,
			t.Album,
			t.ISRC,
			t.SourceID,
		)
	}

	return []models.Track{t}, res.Name, nil
}

func (p *SpotifyParser) parseURL(urlStr string) (spotify.ID, string, error) {
	switch {
	case strings.Contains(urlStr, "/playlist/"):
		return p.extractID(urlStr), "playlist", nil
	case strings.Contains(urlStr, "/album/"):
		return p.extractID(urlStr), "album", nil
	case strings.Contains(urlStr, "/track/"):
		return p.extractID(urlStr), "track", nil
	}
	return "", "", fmt.Errorf("could not identify media type from URL")
}

func (p *SpotifyParser) extractID(urlStr string) spotify.ID {
	parts := strings.Split(urlStr, "/")
	lastPart := parts[len(parts)-1]
	id := strings.Split(lastPart, "?")[0]
	return spotify.ID(id)
}

func (p *SpotifyParser) transform(st spotify.FullTrack) models.Track {
	artists := make([]string, len(st.Artists))
	for i, a := range st.Artists {
		artists[i] = a.Name
	}

	return models.Track{
		Title:    st.Name,
		Artist:   strings.Join(artists, ", "),
		Album:    st.Album.Name,
		ISRC:     st.ExternalIDs["isrc"],
		Type:     "spotify",
		SourceID: string(st.ID),
	}
}