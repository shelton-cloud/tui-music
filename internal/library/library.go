package library

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dhowden/tag"
)

// Song represents a single indexed track in songs.json.
// Field order matches the songs.json contract: path, title, artist, album.
type Song struct {
	Path   string `json:"path"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Album  string `json:"album"`
}

// Track represents an item in the playback queue (either a local Song or a stream).
type Track struct {
	Title   string
	Artist  string
	Album   string
	Source  string
	VideoID string
	IsLocal bool
}

// TrackFromSong converts a library Song into a playable queue Track.
func TrackFromSong(s Song) Track {
	title := s.Title
	if title == "" {
		title = filepath.Base(s.Path)
	}
	artist := s.Artist
	if artist == "" {
		artist = "Unknown Artist"
	}
	album := s.Album
	if album == "" {
		album = "Unknown Album"
	}
	return Track{
		Title:   title,
		Artist:  artist,
		Album:   album,
		Source:  s.Path,
		IsLocal: true,
	}
}

// Load reads and parses songs.json from disk.
func Load(filePath string) ([]Song, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var songs []Song
	if err := json.Unmarshal(data, &songs); err != nil {
		return nil, err
	}
	return songs, nil
}

// Save writes songs to filePath using 4-space indentation matching Python's json.dump(..., indent=4).
func Save(filePath string, songs []Song) error {
	if songs == nil {
		songs = []Song{}
	}
	out, err := json.MarshalIndent(songs, "", "    ")
	if err != nil {
		return err
	}
	// Unescape HTML characters (&, <, >) that Go's json.MarshalIndent escapes by default
	// so output matches Python's json.dump(library, f, indent=4).
	out = bytes.ReplaceAll(out, []byte(`\u0026`), []byte("&"))
	out = bytes.ReplaceAll(out, []byte(`\u003c`), []byte("<"))
	out = bytes.ReplaceAll(out, []byte(`\u003e`), []byte(">"))
	return os.WriteFile(filePath, out, 0o644)
}

var supportedExtensions = map[string]bool{
	".mp3":  true,
	".flac": true,
	".wav":  true,
	".m4a":  true,
	".ogg":  true,
}

// GetMetadata extracts artist, title, and album from an audio file using pure-Go tag reading,
// falling back to the filename and "Unknown Artist" / "Unknown Album" exactly like scanner.py.
func GetMetadata(filePath string) Song {
	meta := Song{
		Path:   filePath,
		Title:  filepath.Base(filePath),
		Artist: "Unknown Artist",
		Album:  "Unknown Album",
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".mp3", ".flac", ".m4a", ".mp4", ".ogg":
		f, err := os.Open(filePath)
		if err != nil {
			return meta
		}
		defer f.Close()

		m, err := tag.ReadFrom(f)
		if err != nil || m == nil {
			return meta
		}
		if t := strings.TrimSpace(m.Title()); t != "" {
			meta.Title = t
		}
		if a := strings.TrimSpace(m.Artist()); a != "" {
			meta.Artist = a
		}
		if al := strings.TrimSpace(m.Album()); al != "" {
			meta.Album = al
		}
	}

	return meta
}

// ScanLibrary walks rootPath for supported audio files and returns their metadata.
// If scanLimit > 0, scanning stops once scanLimit songs have been indexed.
func ScanLibrary(rootPath string, scanLimit int) ([]Song, error) {
	songs := make([]Song, 0)
	fmt.Printf("Scanning %s for music files with rich metadata...\n", rootPath)

	count := 0
	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Continue walking even if an individual directory or file is unreadable.
			return nil
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !supportedExtensions[ext] {
			return nil
		}

		songs = append(songs, GetMetadata(path))
		count++
		if count%10 == 0 {
			fmt.Printf("Indexed %d songs...\n", count)
		}
		if scanLimit > 0 && count >= scanLimit {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return songs, err
	}
	return songs, nil
}

// NodeKind represents the level in the Artist -> Album -> Track library tree.
type NodeKind int

const (
	NodeArtist NodeKind = iota
	NodeAlbum
	NodeTrack
)

// TreeNode represents a node in the Artist -> Album -> Track tree.
type TreeNode struct {
	Kind     NodeKind
	Label    string
	Expanded bool
	Song     *Song
	Children []*TreeNode
}

// FlatNode is a visible row in the flattened tree view.
type FlatNode struct {
	Node  *TreeNode
	Depth int
}

// Tree models the collapsible Artist -> Album -> Track hierarchy derived from songs.json.
type Tree struct {
	Roots   []*TreeNode
	Visible []FlatNode
	Cursor  int
}

// NewTree builds an Artist -> Album -> Track directory tree from a slice of Songs.
// If Artist or Album are missing or "Unknown Artist"/"Unknown Album", it derives them
// from the file path segments when available.
func NewTree(songs []Song) *Tree {
	type albumGroup struct {
		name   string
		tracks []Song
	}
	type artistGroup struct {
		name       string
		albumOrder []string
		albums     map[string]*albumGroup
	}

	artists := make(map[string]*artistGroup)
	var artistOrder []string

	for _, s := range songs {
		artist, album := deriveArtistAlbum(s)
		ag, ok := artists[artist]
		if !ok {
			ag = &artistGroup{
				name:   artist,
				albums: make(map[string]*albumGroup),
			}
			artists[artist] = ag
			artistOrder = append(artistOrder, artist)
		}
		alg, ok := ag.albums[album]
		if !ok {
			alg = &albumGroup{name: album}
			ag.albums[album] = alg
			ag.albumOrder = append(ag.albumOrder, album)
		}
		alg.tracks = append(alg.tracks, s)
	}

	sort.Slice(artistOrder, func(i, j int) bool {
		return strings.ToLower(artistOrder[i]) < strings.ToLower(artistOrder[j])
	})

	roots := make([]*TreeNode, 0, len(artistOrder))
	for _, artistName := range artistOrder {
		ag := artists[artistName]
		sort.Slice(ag.albumOrder, func(i, j int) bool {
			return strings.ToLower(ag.albumOrder[i]) < strings.ToLower(ag.albumOrder[j])
		})
		artistNode := &TreeNode{
			Kind:  NodeArtist,
			Label: artistName,
		}
		for _, albumName := range ag.albumOrder {
			alg := ag.albums[albumName]
			albumNode := &TreeNode{
				Kind:  NodeAlbum,
				Label: albumName,
			}
			for i := range alg.tracks {
				songCopy := alg.tracks[i]
				label := songCopy.Title
				if label == "" {
					label = filepath.Base(songCopy.Path)
				}
				albumNode.Children = append(albumNode.Children, &TreeNode{
					Kind:  NodeTrack,
					Label: label,
					Song:  &songCopy,
				})
			}
			artistNode.Children = append(artistNode.Children, albumNode)
		}
		roots = append(roots, artistNode)
	}

	t := &Tree{Roots: roots}
	t.RebuildVisible()
	return t
}

func deriveArtistAlbum(s Song) (string, string) {
	artist := strings.TrimSpace(s.Artist)
	album := strings.TrimSpace(s.Album)

	// Derive from path (.../Artist/Album/Track.ext) if missing or default placeholder
	cleaned := filepath.ToSlash(filepath.Clean(s.Path))
	parts := strings.Split(cleaned, "/")
	if (artist == "" || artist == "Unknown Artist") && len(parts) >= 3 {
		if candidate := parts[len(parts)-3]; candidate != "" && candidate != "." && candidate != "Volumes" && candidate != "music" {
			artist = candidate
		}
	}
	if (album == "" || album == "Unknown Album") && len(parts) >= 2 {
		if candidate := parts[len(parts)-2]; candidate != "" && candidate != "." && candidate != "Volumes" && candidate != "music" {
			album = candidate
		}
	}
	if artist == "" {
		artist = "Unknown Artist"
	}
	if album == "" {
		album = "Unknown Album"
	}
	return artist, album
}

// RebuildVisible flattens currently expanded tree nodes for display.
func (t *Tree) RebuildVisible() {
	t.Visible = t.Visible[:0]
	var walk func(nodes []*TreeNode, depth int)
	walk = func(nodes []*TreeNode, depth int) {
		for _, n := range nodes {
			t.Visible = append(t.Visible, FlatNode{Node: n, Depth: depth})
			if n.Expanded && len(n.Children) > 0 {
				walk(n.Children, depth+1)
			}
		}
	}
	walk(t.Roots, 0)
	if t.Cursor >= len(t.Visible) {
		t.Cursor = len(t.Visible) - 1
	}
	if t.Cursor < 0 && len(t.Visible) > 0 {
		t.Cursor = 0
	}
}

// MoveUp moves the tree cursor up one row.
func (t *Tree) MoveUp() {
	if t.Cursor > 0 {
		t.Cursor--
	}
}

// MoveDown moves the tree cursor down one row.
func (t *Tree) MoveDown() {
	if t.Cursor+1 < len(t.Visible) {
		t.Cursor++
	}
}

// Activate toggles expansion on artist/album nodes, or returns the selected Song on a track node.
func (t *Tree) Activate() *Song {
	if t.Cursor < 0 || t.Cursor >= len(t.Visible) {
		return nil
	}
	node := t.Visible[t.Cursor].Node
	if node.Kind == NodeTrack {
		return node.Song
	}
	node.Expanded = !node.Expanded
	t.RebuildVisible()
	return nil
}

// Queue manages the ordered playback list and current track index.
type Queue struct {
	Items        []Track
	CurrentIndex int
	MaxCap       int
}

// NewQueue creates a Queue with a default UI cap of 300 tracks (matching app.py).
func NewQueue() *Queue {
	return &Queue{
		CurrentIndex: -1,
		MaxCap:       300,
	}
}

// LoadInitialFromLibrary shuffles a copy of the library and loads up to 200 tracks into the queue.
func (q *Queue) LoadInitialFromLibrary(songs []Song, rng *rand.Rand) {
	if len(songs) == 0 {
		return
	}
	shuffled := make([]Song, len(songs))
	copy(shuffled, songs)
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	rng.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	limit := 200
	if len(shuffled) < limit {
		limit = len(shuffled)
	}
	for i := 0; i < limit; i++ {
		if q.MaxCap > 0 && len(q.Items) >= q.MaxCap {
			break
		}
		q.Items = append(q.Items, TrackFromSong(shuffled[i]))
	}
	if len(q.Items) > 0 && q.CurrentIndex == -1 {
		q.CurrentIndex = 0
	}
}

// Add appends a Track to the queue. If playNow is true and nothing is playing, it selects the new track.
func (q *Queue) Add(track Track, playNow bool) (started bool) {
	if q.MaxCap > 0 && len(q.Items) >= q.MaxCap && !playNow {
		return false
	}
	q.Items = append(q.Items, track)
	if playNow && q.CurrentIndex == -1 {
		q.CurrentIndex = len(q.Items) - 1
		return true
	}
	return false
}

// Current returns the currently active Track, if any.
func (q *Queue) Current() (Track, bool) {
	if q.CurrentIndex >= 0 && q.CurrentIndex < len(q.Items) {
		return q.Items[q.CurrentIndex], true
	}
	return Track{}, false
}

// Select sets CurrentIndex to idx and returns the selected Track.
func (q *Queue) Select(idx int) (Track, bool) {
	if idx < 0 || idx >= len(q.Items) {
		return Track{}, false
	}
	q.CurrentIndex = idx
	return q.Items[idx], true
}

// Next advances to the next track in the queue.
func (q *Queue) Next() (Track, bool) {
	if q.CurrentIndex+1 < len(q.Items) {
		q.CurrentIndex++
		return q.Items[q.CurrentIndex], true
	}
	return Track{}, false
}

// Prev moves to the previous track in the queue.
func (q *Queue) Prev() (Track, bool) {
	if q.CurrentIndex-1 >= 0 {
		q.CurrentIndex--
		return q.Items[q.CurrentIndex], true
	}
	return Track{}, false
}

// Shuffle randomises the queue order while updating CurrentIndex so the currently playing track stays active.
func (q *Queue) Shuffle(rng *rand.Rand) {
	if len(q.Items) == 0 {
		return
	}
	var current Track
	hasCurrent := false
	if q.CurrentIndex >= 0 && q.CurrentIndex < len(q.Items) {
		current = q.Items[q.CurrentIndex]
		hasCurrent = true
	}

	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	rng.Shuffle(len(q.Items), func(i, j int) {
		q.Items[i], q.Items[j] = q.Items[j], q.Items[i]
	})

	if hasCurrent {
		for i, item := range q.Items {
			if tracksMatch(current, item) {
				q.CurrentIndex = i
				return
			}
		}
	} else {
		q.CurrentIndex = 0
	}
}

func tracksMatch(a, b Track) bool {
	if a.IsLocal && b.IsLocal {
		return a.Source == b.Source
	}
	if a.VideoID != "" && b.VideoID != "" {
		return a.VideoID == b.VideoID
	}
	return a.Source == b.Source
}
