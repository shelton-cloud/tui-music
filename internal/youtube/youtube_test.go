package youtube

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseSearchOutputAndFakeBinary(t *testing.T) {
	sampleJSON := `{
  "entries": [
    {
      "id": "dQw4w9WgXcQ",
      "title": "Never Gonna Give You Up",
      "artist": "Rick Astley",
      "duration": 213.0,
      "webpage_url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
    },
    {
      "id": "abc123xyz",
      "title": "Midnight City",
      "uploader": "M83",
      "duration_string": "04:03"
    }
  ]
}`

	tmpDir := t.TempDir()
	fakeScript := filepath.Join(tmpDir, "fake-yt-dlp")
	scriptContent := "#!/bin/sh\ncat << 'EOF'\n" + sampleJSON + "\nEOF\n"
	if err := os.WriteFile(fakeScript, []byte(scriptContent), 0o755); err != nil {
		t.Fatalf("failed to write fake script: %v", err)
	}

	results, err := SearchWithBinary(context.Background(), fakeScript, "synthpop")
	if err != nil {
		t.Fatalf("SearchWithBinary failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].Title != "Never Gonna Give You Up" || results[0].Artist != "Rick Astley" || results[0].Duration != "03:33" {
		t.Fatalf("unexpected first result: %+v", results[0])
	}
	track := results[0].ToTrack()
	if track.Album != "YouTube Music" || track.Source != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" || track.IsLocal {
		t.Fatalf("unexpected converted track: %+v", track)
	}

	if results[1].Artist != "M83" || results[1].Duration != "04:03" || results[1].URL != "https://www.youtube.com/watch?v=abc123xyz" {
		t.Fatalf("unexpected second result: %+v", results[1])
	}
}

func TestInnertubeSearchRadioAndLRC(t *testing.T) {
	searchJSON := `{
  "contents": {
    "tabbedSearchResultsRenderer": {
      "tabs": [
        {
          "musicResponsiveListItemRenderer": {
            "playlistItemData": { "videoId": "vid001" },
            "flexColumns": [
              {
                "musicResponsiveListItemFlexColumnRenderer": {
                  "text": { "runs": [ { "text": "Nightcall" } ] }
                }
              },
              {
                "musicResponsiveListItemFlexColumnRenderer": {
                  "text": {
                    "runs": [
                      { "text": "Song" },
                      { "text": "•" },
                      { "text": "Kavinsky" },
                      { "text": "•" },
                      { "text": "OutRun" },
                      { "text": "•" },
                      { "text": "4:18" }
                    ]
                  }
                }
              }
            ]
          }
        }
      ]
    }
  }
}`
	results, err := ParseInnertubeSearch([]byte(searchJSON))
	if err != nil || len(results) != 1 {
		t.Fatalf("ParseInnertubeSearch failed: res=%+v err=%v", results, err)
	}
	if results[0].VideoID != "vid001" || results[0].Title != "Nightcall" || results[0].Artist != "Kavinsky" || results[0].Album != "OutRun" || results[0].Duration != "4:18" {
		t.Fatalf("unexpected parsed Innertube search result: %+v", results[0])
	}

	radioJSON := `{
  "contents": {
    "playlistPanelRenderer": {
      "contents": [
        {
          "playlistPanelVideoRenderer": {
            "videoId": "vid001",
            "title": { "runs": [ { "text": "Nightcall" } ] }
          }
        },
        {
          "playlistPanelVideoRenderer": {
            "videoId": "vid002",
            "title": { "runs": [ { "text": "Tech Noir" } ] },
            "longBylineText": { "runs": [ { "text": "GUNSHIP • GUNSHIP" } ] },
            "lengthText": { "runs": [ { "text": "4:57" } ] }
          }
        }
      ]
    }
  }
}`
	radioRes, err := ParseInnertubeRadio([]byte(radioJSON), "vid001", 10)
	if err != nil || len(radioRes) != 1 {
		t.Fatalf("ParseInnertubeRadio failed: res=%+v err=%v", radioRes, err)
	}
	if radioRes[0].VideoID != "vid002" || radioRes[0].Title != "Tech Noir" || radioRes[0].Artist != "GUNSHIP" {
		t.Fatalf("unexpected radio track: %+v", radioRes[0])
	}

	lrc := "[ti:Nightcall]\n[00:12.50] I'm giving you a night call\n[01:05.00] To tell you how I feel\n"
	lines := ParseLRC(lrc)
	if len(lines) != 2 || lines[0].TimeSec != 12.5 || lines[1].TimeSec != 65.0 {
		t.Fatalf("unexpected parsed LRC lines: %+v", lines)
	}
}
