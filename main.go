package main

import (
    "context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
    "golang.org/x/oauth2/clientcredentials"
    "github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"

	"dbh-go-srv/internal/dab"
	"dbh-go-srv/internal/database"
	"dbh-go-srv/internal/matcher"
	"dbh-go-srv/internal/models"
	"dbh-go-srv/internal/parser"
)

/* =========================
   Recovery Middleware
   ========================= */

func RecoveryMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("PANIC: %v\n%s", err, debug.Stack())
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next(w, r)
	}
}

/* =========================
   Types
   ========================= */

type ConversionRequest struct {
	URL          string `json:"url"`
	Type         string `json:"type"`
	MatchingMode string `json:"matching_mode"`
}

/* =========================
   SSE Helpers
   ========================= */

func setupSSE(w http.ResponseWriter) (http.Flusher, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming unsupported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	return flusher, nil
}

func sendEvent(w http.ResponseWriter, flusher http.Flusher, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		log.Println("SSE marshal error:", err)
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", b)
	flusher.Flush()
}

/* =========================
   Handler
   ========================= */

func handleConvert(db *sql.DB, sp *parser.SpotifyParser, w http.ResponseWriter, r *http.Request) {
	/* =========================
	   CORS Preflight
	   ========================= */

	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-DAB-Token")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	ctx := r.Context()

	earlyFail := func(msg string, code int) {
		http.Error(w, msg, code)
	}

	/* =========================
	   Auth (NO SSE YET)
	   ========================= */

	token := r.Header.Get("X-DAB-Token")
	if token == "" {
		earlyFail("Missing X-DAB-Token", http.StatusUnauthorized)
		return
	}

	client := dab.GetClient(token)

	userID, err := client.ValidateToken()
	if err != nil {
		earlyFail("Auth failed: "+err.Error(), http.StatusUnauthorized)
		return
	}

	/* =========================
	   Parse Request (NO SSE)
	   ========================= */

	var (
		tracks     []models.Track
		results    []models.MatchResult
		sourceName string
		reqType    string
		matchMode  string
	)

	contentType := r.Header.Get("Content-Type")

	// ---------- CSV (multipart) ----------
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			earlyFail("Invalid multipart form", http.StatusBadRequest)
			return
		}

		reqType = r.FormValue("type")
		matchMode = r.FormValue("matching_mode")

		if reqType != "csv" {
			earlyFail("multipart only supported for type=csv", http.StatusBadRequest)
			return
		}

		// NOTE: CSV parsing does matching internally
		// SSE will be initialized JUST BEFORE calling it

		// defer SSE setup until just before ParseCSV

		/* =========================
		   SSE Setup (SAFE POINT)
		   ========================= */

		flusher, err := setupSSE(w)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		send := func(v any) { sendEvent(w, flusher, v) }

		send(map[string]string{
			"status":  "info",
			"message": "Authenticated",
		})

		onProgress := func(index, total int, res *models.MatchResult) {
			send(map[string]any{
				"status": "processing",
				"index":  index,
				"total":  total,
				"result": res,
			})
		}

		results, sourceName, err = parser.ParseCSV(
			r,
			db,
			client,
			matchMode,
			onProgress,
		)
		if err != nil {
			send(map[string]string{
				"status":  "error",
				"message": "CSV parse failed: " + err.Error(),
			})
			return
		}

		/* ---------- Final ---------- */

		send(map[string]any{
			"status": "complete",
			"meta": map[string]any{
				"user_id":     userID,
				"source_name": sourceName,
				"timestamp":   time.Now().Format(time.RFC3339),
			},
			"tracks": results,
		})
		return
	}

	// ---------- JSON (Spotify / YouTube) ----------

	var req ConversionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		earlyFail("Invalid JSON body", http.StatusBadRequest)
		return
	}

	reqType = req.Type
	matchMode = req.MatchingMode

	parsedURL, err := url.Parse(req.URL)
	if err != nil || parsedURL.Host == "" {
		earlyFail("Invalid URL", http.StatusBadRequest)
		return
	}

	switch reqType {
	case "spotify":
		if !strings.Contains(parsedURL.Host, "spotify.com") &&
			!strings.Contains(parsedURL.Host, "googleusercontent.com") {
			earlyFail("Invalid Spotify URL", http.StatusBadRequest)
			return
		}
		tracks, sourceName, err = sp.Parse(ctx, req.URL) 

	case "youtube":
		if !strings.Contains(parsedURL.Host, "youtube.com") &&
			!strings.Contains(parsedURL.Host, "youtu.be") {
			earlyFail("Invalid YouTube URL", http.StatusBadRequest)
			return
		}
		tracks, sourceName, err = parser.ParseYouTube(req.URL)

	default:
		earlyFail("Unsupported source type", http.StatusBadRequest)
		return
	}

	if err != nil {
		earlyFail("Extraction failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if len(tracks) == 0 {
		earlyFail("No tracks found", http.StatusBadRequest)
		return
	}

	/* =========================
	   SSE Setup (SAFE POINT)
	   ========================= */

	flusher, err := setupSSE(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	send := func(v any) { sendEvent(w, flusher, v) }

	send(map[string]string{
		"status":  "extracting",
		"message": "Parsing " + reqType,
	})

	/* =========================
	   Matching
	   ========================= */

	results = make([]models.MatchResult, 0, len(tracks))

	for i, t := range tracks {
		select {
		case <-ctx.Done():
			log.Println("Client disconnected")
			return
		default:
		}

		res := matcher.MatchTrack(db, client, t, matchMode)
		results = append(results, *res)

		send(map[string]any{
			"status": "processing",
			"index":  i + 1,
			"total":  len(tracks),
			"result": res,
		})
	}

	/* =========================
	   Final
	   ========================= */

	send(map[string]any{
		"status": "complete",
		"meta": map[string]any{
			"user_id":     userID,
			"source_name": sourceName,
			"timestamp":   time.Now().Format(time.RFC3339),
		},
		"tracks": results,
	})
}

/* =========================
   Main
   ========================= */

func main() {
	// 1. Validate Environment Variables (Fail fast)
	spotifyID := os.Getenv("SPOTIFY_ID")
	spotifySecret := os.Getenv("SPOTIFY_SECRET")
	if spotifyID == "" || spotifySecret == "" {
		log.Fatal("CRITICAL: SPOTIFY_ID and SPOTIFY_SECRET must be set in environment")
	}

	// 2. Database Setup
	dbPath := "./data/registry.db"
	_ = os.MkdirAll(filepath.Dir(dbPath), 0755)
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		log.Fatalf("Failed to connect to DB: %v", err)
	}
	defer db.Close()

	if err := database.InitDatabase(db); err != nil {
		log.Fatalf("Failed to init DB schema: %v", err)
	}

	// 3. Initialize Long-Lived Spotify Client
	ctx := context.Background()
	config := &clientcredentials.Config{
		ClientID:     spotifyID,
		ClientSecret: spotifySecret,
		TokenURL:     spotifyauth.TokenURL,
	}
	httpClient := config.Client(ctx)
	spotifyClient := spotify.New(httpClient)

	// 4. Initialize Parsers
    spotifyParser := parser.NewSpotifyParser(spotifyClient)

    // 5. Routing
    http.HandleFunc("/api/v1/convert", RecoveryMiddleware(func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost && r.Method != http.MethodOptions {
            http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
            return
        }
        // PASS the parser instance here
        handleConvert(db, spotifyParser, w, r) 
    }))


	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("DBH Matcher Engine listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}