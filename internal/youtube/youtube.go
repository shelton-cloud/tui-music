package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"tui-music/internal/library"
)

// Result represents a single YouTube search result returned by yt-dlp.
type Result struct {
	VideoID  string
	Title    string
	Artist   string
	Duration string
	URL      string
}

// ToTrack converts a YouTube search Result into a playable queue Track.
func (r Result) ToTrack() library.Track {
	url := r.URL
	if url == "" && r.VideoID != "" {
		url = "https://www.youtube.com/watch?v=" + r.VideoID
	}
	title := r.Title
	if title == "" {
		title = "Unknown"
	}
	artist := r.Artist
	if artist == "" {
		artist = "Unknown"
	}
	return library.Track{
		Title:   title,
		Artist:  artist,
		Album:   "YouTube Music",
		Source:  url,
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

// Search queries YouTube via yt-dlp --dump-single-json "ytsearch5:<query>".
func Search(ctx context.Context, query string) ([]Result, error) {
	bin, err := CheckBinary()
	if err != nil {
		return nil, err
	}
	return SearchWithBinary(ctx, bin, query)
}

// SearchWithBinary runs the specified yt-dlp binary to resolve "ytsearch5:<query>".
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
		"ytsearch5:"+query,
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
		url := firstNonEmpty(e.WebpageURL, e.URL)
		if url == "" || !strings.HasPrefix(url, "http") {
			if e.ID != "" {
				url = "https://www.youtube.com/watch?v=" + e.ID
			}
		}
		results = append(results, Result{
			VideoID:  e.ID,
			Title:    title,
			Artist:   artist,
			Duration: dur,
			URL:      url,
		})
	}
	return results, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
