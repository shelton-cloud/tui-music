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
