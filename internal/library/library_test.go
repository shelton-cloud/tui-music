package library

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadAndSaveSchemaFormatting(t *testing.T) {
	input := `[
    {
        "path": "/Volumes/music/2Pac/All Eyez On Me/California Love.mp3",
        "title": "California Love",
        "artist": "2Pac",
        "album": "All Eyez On Me"
    },
    {
        "path": "/Volumes/music/2Pac (Featuring Dr. Dre & Snoop Dog)/All Eyez On Me/California Love.mp3",
        "title": "California Love",
        "artist": "2Pac (Featuring Dr. Dre & Snoop Dog)",
        "album": "All Eyez On Me"
    }
]`
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "songs.json")
	if err := os.WriteFile(filePath, []byte(input), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	songs, err := Load(filePath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(songs) != 2 {
		t.Fatalf("expected 2 songs, got %d", len(songs))
	}

	outPath := filepath.Join(tmpDir, "songs_out.json")
	if err := Save(outPath, songs); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	savedBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(savedBytes) != input {
		t.Fatalf("saved JSON does not match expected 4-space format.\nExpected:\n%s\nGot:\n%s", input, string(savedBytes))
	}
}

func TestScanLibraryAndFallbacks(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an untagged .mp3 and .flac and a non-audio .txt file
	mp3Path := filepath.Join(tmpDir, "Track01.mp3")
	flacPath := filepath.Join(tmpDir, "Song02.flac")
	txtPath := filepath.Join(tmpDir, "notes.txt")

	if err := os.WriteFile(mp3Path, []byte("not-a-valid-id3-header"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(flacPath, []byte("not-a-valid-flac"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(txtPath, []byte("ignore me"), 0o644); err != nil {
		t.Fatal(err)
	}

	songs, err := ScanLibrary(tmpDir, 0)
	if err != nil {
		t.Fatalf("ScanLibrary failed: %v", err)
	}
	if len(songs) != 2 {
		t.Fatalf("expected 2 indexed songs, got %d", len(songs))
	}
	for _, s := range songs {
		if s.Artist != "Unknown Artist" {
			t.Errorf("expected fallback artist 'Unknown Artist', got %q", s.Artist)
		}
		if s.Album != "Unknown Album" {
			t.Errorf("expected fallback album 'Unknown Album', got %q", s.Album)
		}
		if s.Title != filepath.Base(s.Path) {
			t.Errorf("expected fallback title %q, got %q", filepath.Base(s.Path), s.Title)
		}
	}

	// Test scan limit
	limited, err := ScanLibrary(tmpDir, 1)
	if err != nil {
		t.Fatalf("ScanLibrary with limit failed: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected 1 song with limit=1, got %d", len(limited))
	}
}

func TestGetMetadataID3v1(t *testing.T) {
	tmpDir := t.TempDir()
	mp3Path := filepath.Join(tmpDir, "tagged.mp3")

	// Construct a minimal 128-byte ID3v1 tag
	buf := make([]byte, 128)
	copy(buf[0:3], []byte("TAG"))
	copy(buf[3:33], []byte("Synthwave Nights"))
	copy(buf[33:63], []byte("Retro Runner"))
	copy(buf[63:93], []byte("CRT Dreams"))
	buf[127] = 255

	if err := os.WriteFile(mp3Path, buf, 0o644); err != nil {
		t.Fatal(err)
	}

	meta := GetMetadata(mp3Path)
	if !strings.HasPrefix(meta.Title, "Synthwave Nights") {
		t.Fatalf("expected title 'Synthwave Nights', got %q", meta.Title)
	}
	if !strings.HasPrefix(meta.Artist, "Retro Runner") {
		t.Fatalf("expected artist 'Retro Runner', got %q", meta.Artist)
	}
	if !strings.HasPrefix(meta.Album, "CRT Dreams") {
		t.Fatalf("expected album 'CRT Dreams', got %q", meta.Album)
	}
}

func TestTreeNavigationAndActivation(t *testing.T) {
	songs := []Song{
		{
			Path:   "/Volumes/music/2Pac/All Eyez On Me/California Love.mp3",
			Title:  "California Love",
			Artist: "2Pac",
			Album:  "All Eyez On Me",
		},
		{
			Path:   "/Volumes/music/Daft Punk/Discovery/One More Time.mp3",
			Title:  "One More Time",
			Artist: "Daft Punk",
			Album:  "Discovery",
		},
	}

	tree := NewTree(songs)
	if len(tree.Visible) != 2 {
		t.Fatalf("expected 2 collapsed root artist nodes, got %d", len(tree.Visible))
	}

	// Activate first artist ("2Pac") -> expands album "All Eyez On Me"
	if song := tree.Activate(); song != nil {
		t.Fatalf("expected nil song when expanding artist node, got %+v", song)
	}
	if len(tree.Visible) != 3 {
		t.Fatalf("expected 3 visible nodes after expanding artist, got %d", len(tree.Visible))
	}

	// Move down to album node and expand it
	tree.MoveDown()
	if song := tree.Activate(); song != nil {
		t.Fatalf("expected nil song when expanding album node, got %+v", song)
	}
	if len(tree.Visible) != 4 {
		t.Fatalf("expected 4 visible nodes after expanding album, got %d", len(tree.Visible))
	}

	// Move down to track node and activate it
	tree.MoveDown()
	selected := tree.Activate()
	if selected == nil || selected.Title != "California Love" {
		t.Fatalf("expected track 'California Love', got %+v", selected)
	}
}

func TestQueueOperationsAndShuffle(t *testing.T) {
	var songs []Song
	for i := 0; i < 250; i++ {
		songs = append(songs, Song{
			Path:   filepath.Join("/Volumes/music", "Track_"+string(rune('A'+(i%26)))+"_"+string(rune('0'+(i/26)))+".mp3"),
			Title:  "Song",
			Artist: "Artist",
			Album:  "Album",
		})
	}

	q := NewQueue()
	rng := rand.New(rand.NewSource(42))
	q.LoadInitialFromLibrary(songs, rng)

	if len(q.Items) != 200 {
		t.Fatalf("expected initial queue load of 200 items, got %d", len(q.Items))
	}
	if q.CurrentIndex != 0 {
		t.Fatalf("expected CurrentIndex 0, got %d", q.CurrentIndex)
	}

	// Select track 5
	active, ok := q.Select(5)
	if !ok {
		t.Fatalf("Select(5) failed")
	}

	// Shuffle and ensure CurrentIndex follows the active track
	q.Shuffle(rng)
	afterShuffle, ok := q.Current()
	if !ok || afterShuffle.Source != active.Source {
		t.Fatalf("expected current track after shuffle to stay %q, got %q", active.Source, afterShuffle.Source)
	}

	// Test Next and Prev
	prevIdx := q.CurrentIndex
	if prevIdx+1 < len(q.Items) {
		if _, ok := q.Next(); !ok || q.CurrentIndex != prevIdx+1 {
			t.Fatalf("Next() did not advance CurrentIndex properly")
		}
		if _, ok := q.Prev(); !ok || q.CurrentIndex != prevIdx {
			t.Fatalf("Prev() did not decrement CurrentIndex properly")
		}
	}
}

func TestResolveSongPathCrossOSAndFavorites(t *testing.T) {
	linuxRoot := "/run/user/1000/gvfs/smb-share:server=nasa.local,share=music"
	macPath := "/Volumes/music/2Pac/All Eyez On Me/California Love.mp3"

	got := ResolveSongPath(macPath, linuxRoot)
	want := filepath.Join(linuxRoot, "2Pac", "All Eyez On Me", "California Love.mp3")
	if got != want {
		t.Fatalf("ResolveSongPath(%q, %q) = %q, want %q", macPath, linuxRoot, got, want)
	}

	gotMac := ResolveSongPath(want, "/Volumes/music")
	if gotMac != macPath {
		t.Fatalf("ResolveSongPath(%q, /Volumes/music) = %q, want %q", want, gotMac, macPath)
	}

	tmpDir := t.TempDir()
	t.Setenv("TUI_MUSIC_ROOT", tmpDir)
	if detected := DetectMusicRoot(); detected != tmpDir {
		t.Fatalf("DetectMusicRoot() = %q, want %q", detected, tmpDir)
	}

	// Test Favorites persistence and ToggleFavorite
	favPath := filepath.Join(tmpDir, "favorites.json")
	ytTrack := Track{
		Title:   "Midnight City",
		Artist:  "M83",
		Album:   "Hurry Up, We're Dreaming",
		Source:  "https://www.youtube.com/watch?v=dX3k_QDnzHE",
		VideoID: "dX3k_QDnzHE",
		IsLocal: false,
	}
	favs, added := ToggleFavorite(nil, ytTrack)
	if !added || len(favs) != 1 || !IsFavorite(favs, ytTrack) {
		t.Fatalf("expected track to be added to favorites")
	}
	if err := SaveFavorites(favPath, favs); err != nil {
		t.Fatalf("SaveFavorites failed: %v", err)
	}
	loadedFavs, err := LoadFavorites(favPath)
	if err != nil || len(loadedFavs) != 1 || loadedFavs[0].VideoID != "dX3k_QDnzHE" {
		t.Fatalf("LoadFavorites mismatch: %+v, err=%v", loadedFavs, err)
	}
	favs, added = ToggleFavorite(loadedFavs, ytTrack)
	if added || len(favs) != 0 {
		t.Fatalf("expected second ToggleFavorite to remove track")
	}

	// Ensure ResolveSongs resolves 10,000+ NAS paths without per-file stat overhead
	large := make([]Song, 10000)
	for i := range large {
		large[i] = Song{
			Path:   "/Volumes/music/Artist/Album/Track.mp3",
			Title:  "Track",
			Artist: "Artist",
			Album:  "Album",
		}
	}
	start := time.Now()
	resolved := ResolveSongs(large, tmpDir)
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("expected ResolveSongs on 10k songs to complete without disk I/O (<200ms), took %v", elapsed)
	}
	wantResolved := filepath.Join(tmpDir, "Artist", "Album", "Track.mp3")
	if resolved[0].Path != wantResolved {
		t.Fatalf("expected resolved path %q, got %q", wantResolved, resolved[0].Path)
	}
}
