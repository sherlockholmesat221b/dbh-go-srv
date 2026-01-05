# DBH-GO-SRV

A Go-based DABHounds web server designed to convert music playlists from **Spotify** and **YouTube** into **DAB/Qobuz** compatible IDs. It features real-time progress streaming via Server-Sent Events (SSE), an SQLite-backed ID registry, and metadata enrichment via MusicBrainz.

## 🚀 Features

* **Real-time Streaming**: Uses SSE to send per-track matching results to the client immediately.
* **Dual-Layer Matching**: Searches Qobuz first for high-fidelity matches, falling back to DAB internal search.
* **Registry Persistence**: Maps Spotify/YouTube IDs to DAB IDs in a local SQLite database to prevent redundant API calls.
* **ISRC Pivot**: Automatically fetches ISRCs from MusicBrainz for YouTube tracks to ensure 100% matching accuracy.
* **Rate Limited**: Respects external API limits (1.5 req/s for DAB, 1 req/s for MusicBrainz).

---

## 🛠 Setup

### 1. Prerequisites
* Go 1.21 or higher
* Spotify Developer Credentials

### 2. Installation
```
git clone https://github.com/sherlockholmesat221b/dbh-go-srv
cd dbh-go-srv
go mod tidy
go build -o din/srv ./
```

### 3. Configuration
Create a `.env` file in the root directory
```env
SPOTIFY_ID=your_spotify_client_id
SPOTIFY_SECRET=your_spotify_client_secret
QOBUZ_APP_ID=000000000
PORT=8080
```

---

## 📡 API Reference

### Convert Playlist/Track
`POST /api/v1/convert`

**Headers:**
* `X-DAB-Token`: Your DAB session/auth token.
* `Content-Type`: `application/json`

**Body:**
    {
        "url": "https://open.spotify.com/playlist/...",
        "type": "spotify",
        "matching_mode": "strict"
    }

**Response (SSE Stream):**
The server returns `text/event-stream`. Each event is a JSON object:
    data: {"status":"processing","index":1,"total":20,"result":{...}}
    data: {"status":"complete","meta":{...},"tracks":[...]}

---

## 📂 Accepted CSV Format

If you use the CSV import feature, the file must be a `.csv` with a header row. The engine expects columns in the following order:

    title,artist,album,isrc
    "Blinding Lights","The Weeknd","After Hours","USUM71921131"
    "Levitating","Dua Lipa","Future Nostalgia","GBAYE2000674"

* **Title & Artist**: Required.
* **Album**: Optional (helps fuzzy matching).
* **ISRC**: Optional (if provided, matching is near-instant and 100% accurate).

---

## 🏗 Project Structure



* `internal/dab`: DABMusic API client with rate limiting and session validation.
* `internal/database`: SQLite schema and ID mapping registry.
* `internal/matcher`: The matching engine.
* `internal/parser`: Logic for scraping/fetching data from Spotify, YouTube, and CSVs.
* `main.go`: HTTP server and SSE orchestration.

---

## 🧠 Matching Logic Flow

1.  **Registry Check**: Does this `spotify_id` or `youtube_id` already exist in `registry.db`? If yes, return immediately.
2.  **Metadata Enrichment**: 
    * If Spotify: Use the provided ISRC.
    * If YouTube: Use `NormalizeYTTitle` + MusicBrainz to find the ISRC.
3.  **Source Search**: Search Qobuz/DAB using the ISRC (or Artist/Title fuzzy search).
4.  **Fuzzy Scoring**: Use Jaro-Winkler distance to verify the match quality.
5.  **Cache & Stream**: Save the new mapping to the Registry and stream the result to the UI.

## ⚖️ License
GNU Alfero General Public License v3