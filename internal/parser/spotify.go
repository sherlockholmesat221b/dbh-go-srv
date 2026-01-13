package parser

import (
	"context"
	"fmt"
	"strings"

	"dbh-go-srv/internal/models"
	"dbh-go-srv/internal/spotifetch"
	"github.com/zmb3/spotify/v2"
)

type SpotifyParser struct {
	client *spotify.Client
}

func NewSpotifyParser(client *spotify.Client) *SpotifyParser {
	return &SpotifyParser{client: client}
}

// Parse tries SpotiFLAC first, then falls back to official Spotify API
func (p *SpotifyParser) Parse(ctx context.Context, url string) ([]models.Track, string, error) {
	// --- Step 1: Try SpotiFLAC metadata fetch ---
	meta, err := spotifetch.GetFilteredSpotifyData(ctx, url, false, 0)
	if err == nil {
		tracks, name := convertMetadataToTracks(meta)
		return tracks, name, nil
	}

	// --- Step 2: Fallback to official Spotify Web API ---
	id, mediaType, err := p.parseURL(url)
	if err != nil {
		return nil, "", fmt.Errorf("spotify parse url: %w", err)
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
			Artist:   t.Artists, // Already a string in spotifetch
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
		return tracks, v.PlaylistInfo.Owner.Name // Or v.PlaylistInfo.Description
	}

	return nil, ""
}

// --- Original Web API functions ---
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
				tracks = append(tracks, p.transform(item.Track))
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
			tracks = append(tracks, p.transform(*ft))
		}
	}

	return tracks, res.Name, nil
}

func (p *SpotifyParser) handleTrack(ctx context.Context, id spotify.ID) ([]models.Track, string, error) {
	res, err := p.client.GetTrack(ctx, id)
	if err != nil {
		return nil, "", fmt.Errorf("get track: %w", err)
	}
	return []models.Track{p.transform(*res)}, res.Name, nil
}

func (p *SpotifyParser) parseURL(urlStr string) (spotify.ID, string, error) {
	if strings.Contains(urlStr, "/playlist/") {
		return p.extractID(urlStr), "playlist", nil
	} else if strings.Contains(urlStr, "/album/") {
		return p.extractID(urlStr), "album", nil
	} else if strings.Contains(urlStr, "/track/") {
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