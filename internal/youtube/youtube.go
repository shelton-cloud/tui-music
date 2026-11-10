package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"tui-music/internal/library"
)

// Default endpoints (overridable in unit tests).
var (
	InnertubeSearchURL = "https://music.youtube.com/youtubei/v1/search?prettyPrint=false"
	InnertubeNextURL   = "https://music.youtube.com/youtubei/v1/next?prettyPrint=false"
	LRCLibBaseURL      = "https://lrclib.net/api"
)

// Result represents a single YouTube Music search or radio result.
type Result struct {
	VideoID  string
	Title    string
	Artist   string
	Album    string
	Duration string
	URL      string
}

// ToTrack converts a YouTube search Result into a playable queue Track.
func (r Result) ToTrack() library.Track {
	u := r.URL
	if u == "" && r.VideoID != "" {
		u = "https://www.youtube.com/watch?v=" + r.VideoID
	}
	title := r.Title
	if title == "" {
		title = "Unknown"
	}
	artist := r.Artist
	if artist == "" {
		artist = "Unknown"
	}
	album := r.Album
	if album == "" {
		album = "YouTube Music"
	}
	return library.Track{
		Title:   title,
		Artist:  artist,
		Album:   album,
		Source:  u,
		VideoID: r.VideoID,
		IsLocal: false,
	}
}

// CheckBinary verifies that the yt-dlp executable is available in PATH.
func CheckBinary() (string, error) {
	p, err := exec.LookPath("yt-dlp")
	if err != nil {
		installHint := "sudo apt install yt-dlp (or pip install yt-dlp)"
		if runtime.GOOS == "darwin" {
			installHint = "brew install yt-dlp"
		}
		return "", fmt.Errorf("required binary 'yt-dlp' not found in PATH; install it with: %s", installHint)
	}
	return p, nil
}

// Search queries YouTube Music via its fast Innertube API first, falling back to yt-dlp if needed.
func Search(ctx context.Context, query string) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if results, err := SearchInnertube(ctx, InnertubeSearchURL, query); err == nil && len(results) > 0 {
		return results, nil
	}
	bin, err := CheckBinary()
	if err != nil {
		return nil, err
	}
	return SearchWithBinary(ctx, bin, query)
}

// SearchInnertube queries the YouTube Music WEB_REMIX Innertube search endpoint directly over HTTP.
func SearchInnertube(ctx context.Context, endpointURL, query string) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	// Params "EgWKAQIIAWoKEAkQBRAKEAMQBA==" filters YouTube Music search specifically to Songs.
	payload := map[string]any{
		"context": map[string]any{
			"client": map[string]any{
				"clientName":    "WEB_REMIX",
				"clientVersion": "1.20240918.01.00",
				"hl":            "en",
				"gl":            "US",
			},
		},
		"query":  query,
		"params": "EgWKAQIIAWoKEAkQBRAKEAMQBA==",
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Origin", "https://music.youtube.com")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("innertube search HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return ParseInnertubeSearch(data)
}

// ParseInnertubeSearch extracts song Results from a YouTube Music Innertube search response.
func ParseInnertubeSearch(data []byte) ([]Result, error) {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	var results []Result
	seen := make(map[string]bool)

	var walk func(node any)
	walk = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			if renderer, ok := v["musicResponsiveListItemRenderer"].(map[string]any); ok {
				if res, ok := parseResponsiveListItem(renderer); ok && !seen[res.VideoID] {
					seen[res.VideoID] = true
					results = append(results, res)
				}
				return
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		}
	}
	walk(root)
	return results, nil
}

var durationPattern = regexp.MustCompile(`^\d{1,2}:\d{2}(:\d{2})?$`)

func parseResponsiveListItem(renderer map[string]any) (Result, bool) {
	var videoID string
	if playlistData, ok := renderer["playlistItemData"].(map[string]any); ok {
		videoID, _ = playlistData["videoId"].(string)
	}
	if videoID == "" {
		return Result{}, false
	}

	flexCols, _ := renderer["flexColumns"].([]any)
	var title string
	var metaRuns []string
	for idx, col := range flexCols {
		colMap, _ := col.(map[string]any)
		colRenderer, _ := colMap["musicResponsiveListItemFlexColumnRenderer"].(map[string]any)
		textObj, _ := colRenderer["text"].(map[string]any)
		runs, _ := textObj["runs"].([]any)
		for _, r := range runs {
			runMap, _ := r.(map[string]any)
			txt, _ := runMap["text"].(string)
			txt = strings.TrimSpace(txt)
			if txt == "" || txt == "•" || txt == "," || txt == "&" {
				continue
			}
			if idx == 0 && title == "" {
				title = txt
			} else if idx > 0 {
				metaRuns = append(metaRuns, txt)
			}
		}
	}
	if title == "" {
		return Result{}, false
	}

	artist := "Unknown"
	album := "YouTube Music"
	duration := "??:??"
	for _, token := range metaRuns {
		if durationPattern.MatchString(token) {
			duration = token
			continue
		}
		if strings.EqualFold(token, "Song") || strings.EqualFold(token, "Video") {
			continue
		}
		if artist == "Unknown" {
			artist = token
		} else if album == "YouTube Music" {
			album = token
		}
	}

	return Result{
		VideoID:  videoID,
		Title:    title,
		Artist:   artist,
		Album:    album,
		Duration: duration,
		URL:      "https://www.youtube.com/watch?v=" + videoID,
	}, true
}

// FetchRadio fetches an "Up Next" YouTube Music radio queue (RDAMVM<videoID>) for a seed videoID.
func FetchRadio(ctx context.Context, videoID string, limit int) ([]Result, error) {
	videoID = strings.TrimSpace(videoID)
	if videoID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if results, err := FetchRadioInnertube(ctx, InnertubeNextURL, videoID, limit); err == nil && len(results) > 0 {
		return results, nil
	}
	bin, err := CheckBinary()
	if err != nil {
		return nil, err
	}
	return FetchRadioWithBinary(ctx, bin, videoID, limit)
}

// FetchRadioInnertube queries the YouTube Music Innertube `next` endpoint for RDAMVM<videoID> radio tracks.
func FetchRadioInnertube(ctx context.Context, endpointURL, videoID string, limit int) ([]Result, error) {
	if videoID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	payload := map[string]any{
		"context": map[string]any{
			"client": map[string]any{
				"clientName":    "WEB_REMIX",
				"clientVersion": "1.20240918.01.00",
				"hl":            "en",
				"gl":            "US",
			},
		},
		"videoId":     videoID,
		"playlistId":  "RDAMVM" + videoID,
		"isAudioOnly": true,
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Origin", "https://music.youtube.com")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("innertube next HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return ParseInnertubeRadio(data, videoID, limit)
}

// ParseInnertubeRadio parses playlistPanelVideoRenderer items from an Innertube `next` response.
func ParseInnertubeRadio(data []byte, seedVideoID string, limit int) ([]Result, error) {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	var results []Result
	seen := map[string]bool{seedVideoID: true}

	var walk func(node any)
	walk = func(node any) {
		if len(results) >= limit {
			return
		}
		switch v := node.(type) {
		case map[string]any:
			if renderer, ok := v["playlistPanelVideoRenderer"].(map[string]any); ok {
				if res, ok := parsePlaylistPanelRenderer(renderer); ok && !seen[res.VideoID] {
					seen[res.VideoID] = true
					results = append(results, res)
				}
				return
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		}
	}
	walk(root)
	return results, nil
}

func parsePlaylistPanelRenderer(renderer map[string]any) (Result, bool) {
	vid, _ := renderer["videoId"].(string)
	if vid == "" {
		return Result{}, false
	}
	title := extractRunsText(renderer["title"])
	if title == "" {
		return Result{}, false
	}
	byline := extractRunsText(renderer["longBylineText"])
	if byline == "" {
		byline = extractRunsText(renderer["shortBylineText"])
	}
	artist := "Unknown"
	album := "YouTube Music"
	if byline != "" {
		parts := strings.Split(byline, "•")
		if len(parts) > 0 && strings.TrimSpace(parts[0]) != "" {
			artist = strings.TrimSpace(parts[0])
		}
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
			album = strings.TrimSpace(parts[1])
		}
	}
	dur := extractRunsText(renderer["lengthText"])
	if dur == "" {
		dur = "??:??"
	}
	return Result{
		VideoID:  vid,
		Title:    title,
		Artist:   artist,
		Album:    album,
		Duration: dur,
		URL:      "https://www.youtube.com/watch?v=" + vid,
	}, true
}

func extractRunsText(field any) string {
	m, ok := field.(map[string]any)
	if !ok {
		return ""
	}
	runs, ok := m["runs"].([]any)
	if !ok {
		return ""
	}
	var sb strings.Builder
	for _, r := range runs {
		if rm, ok := r.(map[string]any); ok {
			if txt, ok := rm["text"].(string); ok {
				sb.WriteString(txt)
			}
		}
	}
	return strings.TrimSpace(sb.String())
}

// FetchRadioWithBinary uses yt-dlp to extract the RDAMVM radio mix for a seed videoID.
func FetchRadioWithBinary(ctx context.Context, binPath, videoID string, limit int) ([]Result, error) {
	if videoID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	mixURL := fmt.Sprintf("https://music.youtube.com/watch?v=%s&list=RDAMVM%s", videoID, videoID)
	cmd := exec.CommandContext(
		ctx,
		binPath,
		"--dump-single-json",
		"--flat-playlist",
		"--no-warnings",
		"--playlist-end", strconv.Itoa(limit+1),
		mixURL,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp radio fetch failed: %w", err)
	}
	parsed, err := ParseSearchOutput(out)
	if err != nil {
		return nil, err
	}
	filtered := make([]Result, 0, len(parsed))
	for _, r := range parsed {
		if r.VideoID == videoID {
			continue
		}
		filtered = append(filtered, r)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered, nil
}

// SearchWithBinary runs the specified yt-dlp binary to resolve "ytsearch10:<query>".
func SearchWithBinary(ctx context.Context, binPath, query string) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		binPath,
		"--dump-single-json",
		"--flat-playlist",
		"--no-warnings",
		"ytsearch10:"+query,
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp search failed: %w", err)
	}
	return ParseSearchOutput(out)
}

type ytdlpPlaylist struct {
	Entries []ytdlpEntry `json:"entries"`
}

type ytdlpEntry struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Artist         string   `json:"artist"`
	Creator        string   `json:"creator"`
	Uploader       string   `json:"uploader"`
	Channel        string   `json:"channel"`
	Duration       *float64 `json:"duration"`
	DurationString string   `json:"duration_string"`
	WebpageURL     string   `json:"webpage_url"`
	URL            string   `json:"url"`
}

// ParseSearchOutput parses the JSON emitted by `yt-dlp --dump-single-json`.
func ParseSearchOutput(data []byte) ([]Result, error) {
	var pl ytdlpPlaylist
	if err := json.Unmarshal(data, &pl); err != nil {
		return nil, fmt.Errorf("failed to parse yt-dlp JSON: %w", err)
	}

	results := make([]Result, 0, len(pl.Entries))
	for _, e := range pl.Entries {
		if e.ID == "" && e.Title == "" {
			continue
		}
		artist := firstNonEmpty(e.Artist, e.Creator, e.Uploader, e.Channel, "Unknown")
		title := firstNonEmpty(e.Title, "Unknown")
		dur := e.DurationString
		if dur == "" {
			if e.Duration != nil && *e.Duration > 0 {
				totalSec := int(math.Round(*e.Duration))
				dur = fmt.Sprintf("%02d:%02d", totalSec/60, totalSec%60)
			} else {
				dur = "??:??"
			}
		}
		u := firstNonEmpty(e.WebpageURL, e.URL)
		if u == "" || !strings.HasPrefix(u, "http") {
			if e.ID != "" {
				u = "https://www.youtube.com/watch?v=" + e.ID
			}
		}
		results = append(results, Result{
			VideoID:  e.ID,
			Title:    title,
			Artist:   artist,
			Duration: dur,
			URL:      u,
		})
	}
	return results, nil
}

// LyricLine represents a single timestamped or plain lyric line.
type LyricLine struct {
	TimeSec float64
	Text    string
}

var lrcLineRegex = regexp.MustCompile(`^\[(\d{1,3}):(\d{2}(?:\.\d{1,3})?)\]\s*(.*)$`)

// ParseLRC parses synced LRC text (or falls back to plain lines when timestamps are absent).
func ParseLRC(lrcText string) []LyricLine {
	rawLines := strings.Split(strings.ReplaceAll(lrcText, "\r\n", "\n"), "\n")
	var lines []LyricLine
	for _, raw := range rawLines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if matches := lrcLineRegex.FindStringSubmatch(trimmed); len(matches) == 4 {
			mins, err1 := strconv.ParseFloat(matches[1], 64)
			secs, err2 := strconv.ParseFloat(matches[2], 64)
			txt := strings.TrimSpace(matches[3])
			if err1 == nil && err2 == nil {
				if txt == "" {
					txt = "♪"
				}
				lines = append(lines, LyricLine{
					TimeSec: mins*60 + secs,
					Text:    txt,
				})
				continue
			}
		}
		// Skip LRC metadata tags like [ar:...], [ti:...]
		if strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, ":") && strings.HasSuffix(trimmed, "]") {
			continue
		}
		lines = append(lines, LyricLine{
			TimeSec: -1,
			Text:    trimmed,
		})
	}
	return lines
}

// FetchLyrics queries lrclib.net for time-synced or plain lyrics matching title and artist.
func FetchLyrics(ctx context.Context, title, artist string) ([]LyricLine, error) {
	title = strings.TrimSpace(title)
	artist = strings.TrimSpace(artist)
	if title == "" || title == "No Track Playing" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	params := url.Values{}
	params.Set("track_name", title)
	if artist != "" && artist != "Unknown Artist" && artist != "Unknown" {
		params.Set("artist_name", artist)
	}
	endpoint := strings.TrimRight(LRCLibBaseURL, "/") + "/get?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "tuimusic/1.0 (https://github.com/shelton/tui-music)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var payload struct {
			SyncedLyrics string `json:"syncedLyrics"`
			PlainLyrics  string `json:"plainLyrics"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil {
			if strings.TrimSpace(payload.SyncedLyrics) != "" {
				return ParseLRC(payload.SyncedLyrics), nil
			}
			if strings.TrimSpace(payload.PlainLyrics) != "" {
				return ParseLRC(payload.PlainLyrics), nil
			}
		}
	}

	// Fallback: search lrclib.net/api/search?q=<title> <artist>
	q := title
	if artist != "" && artist != "Unknown Artist" && artist != "Unknown" {
		q += " " + artist
	}
	searchEndpoint := strings.TrimRight(LRCLibBaseURL, "/") + "/search?q=" + url.QueryEscape(q)
	sReq, err := http.NewRequestWithContext(ctx, http.MethodGet, searchEndpoint, nil)
	if err != nil {
		return nil, err
	}
	sReq.Header.Set("User-Agent", "tuimusic/1.0 (https://github.com/shelton/tui-music)")
	sResp, err := http.DefaultClient.Do(sReq)
	if err != nil {
		return nil, err
	}
	defer sResp.Body.Close()
	if sResp.StatusCode != http.StatusOK {
		return nil, nil
	}
	var hits []struct {
		SyncedLyrics string `json:"syncedLyrics"`
		PlainLyrics  string `json:"plainLyrics"`
	}
	if err := json.NewDecoder(sResp.Body).Decode(&hits); err != nil || len(hits) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(hits[0].SyncedLyrics) != "" {
		return ParseLRC(hits[0].SyncedLyrics), nil
	}
	if strings.TrimSpace(hits[0].PlainLyrics) != "" {
		return ParseLRC(hits[0].PlainLyrics), nil
	}
	return nil, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
