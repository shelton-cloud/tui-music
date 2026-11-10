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
	Title   string `json:"title"`
	Artist  string `json:"artist"`
	Album   string `json:"album"`
	Source  string `json:"source"`
	VideoID string `json:"video_id,omitempty"`
	IsLocal bool   `json:"is_local"`
}

// knownNASPrefixes lists common macOS and Linux NAS mount prefixes that may appear in songs.json.
var knownNASPrefixes = []string{
	"/Volumes/music/",
	"/mnt/music/",
	"/mnt/nasa/music/",
	"/mnt/nas/music/",
	"/media/music/",
}

// DetectMusicRoot locates the active NAS music directory across Linux and macOS.
// It checks TUI_MUSIC_ROOT first, then common GVFS SMB/NFS mounts for nasa.local/music,
// standard Linux /mnt and /media paths, and macOS /Volumes/music.
func DetectMusicRoot() string {
	if envRoot := strings.TrimSpace(os.Getenv("TUI_MUSIC_ROOT")); envRoot != "" {
		if info, err := os.Stat(envRoot); err == nil && info.IsDir() {
			return envRoot
		}
	}

	var gvfsDirs []string
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		gvfsDirs = append(gvfsDirs, filepath.Join(xdg, "gvfs"))
	}
	gvfsDirs = append(gvfsDirs, filepath.Join("/run/user", fmt.Sprintf("%d", os.Getuid()), "gvfs"))

	var candidates []string
	for _, gvfsDir := range gvfsDirs {
		candidates = append(candidates,
			filepath.Join(gvfsDir, "smb-share:server=nasa.local,share=music"),
			filepath.Join(gvfsDir, "smb-share:server=nasa,share=music"),
			filepath.Join(gvfsDir, "nfs:host=nasa.local,prefix=/music"),
		)
		if entries, err := os.ReadDir(gvfsDir); err == nil {
			for _, e := range entries {
				nameLower := strings.ToLower(e.Name())
				entryPath := filepath.Join(gvfsDir, e.Name())
				if strings.Contains(nameLower, "nasa") || strings.Contains(nameLower, "music") {
					candidates = append(candidates, filepath.Join(entryPath, "music"), entryPath)
				}
			}
		}
	}

	user := os.Getenv("USER")
	if user == "" {
		user = "shelton"
	}
	home, _ := os.UserHomeDir()

	candidates = append(candidates,
		"/mnt/nasa/music",
		"/mnt/music",
		"/mnt/nas/music",
		filepath.Join("/media", user, "music"),
		filepath.Join("/media", user, "nasa", "music"),
		"/media/music",
		"/Volumes/music",
	)
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, "nasa", "music"),
			filepath.Join(home, "music"),
			filepath.Join(home, "Music", "nasa"),
		)
	}

	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}

	return "/Volumes/music"
}

// ResolveSongPath maps a path stored in songs.json (e.g., "/Volumes/music/Artist/Album/Track.mp3")
// onto the active local/NAS musicRoot when running across different operating systems.
func ResolveSongPath(rawPath, musicRoot string) string {
	if rawPath == "" || musicRoot == "" {
		return rawPath
	}
	if _, err := os.Stat(rawPath); err == nil {
		return rawPath
	}

	slashPath := filepath.ToSlash(rawPath)
	for _, prefix := range knownNASPrefixes {
		if strings.HasPrefix(slashPath, prefix) {
			rel := strings.TrimPrefix(slashPath, prefix)
			return filepath.Join(musicRoot, filepath.FromSlash(rel))
		}
	}

	if idx := strings.Index(slashPath, "/gvfs/"); idx != -1 {
		rest := slashPath[idx+len("/gvfs/"):]
		if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
			rel := strings.TrimPrefix(rest[slashIdx+1:], "music/")
			return filepath.Join(musicRoot, filepath.FromSlash(rel))
		}
	}

	return rawPath
}

// ResolveSongs returns a slice of Songs with paths mapped to musicRoot if needed.
func ResolveSongs(songs []Song, musicRoot string) []Song {
	if musicRoot == "" {
		return songs
	}
	resolved := make([]Song, len(songs))
	for i, s := range songs {
		s.Path = ResolveSongPath(s.Path, musicRoot)
		resolved[i] = s
	}
	return resolved
}

// IsNASAvailable checks whether the musicRoot directory exists or any sample song path in songs is reachable.
func IsNASAvailable(songs []Song, musicRoot string) bool {
	if musicRoot != "" {
		if info, err := os.Stat(musicRoot); err == nil && info.IsDir() {
			return true
		}
	}
	limit := 10
	if len(songs) < limit {
		limit = len(songs)
	}
	for i := 0; i < limit; i++ {
		if _, err := os.Stat(songs[i].Path); err == nil {
			return true
		}
	}
	return false
}

// DefaultFavoritesPath returns the user config path for storing hybrid YT/NAS favorite tracks.
func DefaultFavoritesPath() string {
	if envPath := strings.TrimSpace(os.Getenv("TUI_MUSIC_FAVORITES")); envPath != "" {
		return envPath
	}
	if cfgDir, err := os.UserConfigDir(); err == nil && cfgDir != "" {
		return filepath.Join(cfgDir, "tuimusic", "favorites.json")
	}
	return "favorites.json"
}

// LoadFavorites reads saved favorite tracks from disk.
func LoadFavorites(filePath string) ([]Track, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var tracks []Track
	if err := json.Unmarshal(data, &tracks); err != nil {
		return nil, err
	}
	return tracks, nil
}

// SaveFavorites writes favorite tracks to disk, creating parent directories if needed.
func SaveFavorites(filePath string, tracks []Track) error {
	if tracks == nil {
		tracks = []Track{}
	}
	if dir := filepath.Dir(filePath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	out, err := json.MarshalIndent(tracks, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, out, 0o644)
}

// IsFavorite checks whether target is present in the favorites slice.
func IsFavorite(favorites []Track, target Track) bool {
	for _, f := range favorites {
		if TracksMatch(f, target) {
			return true
		}
	}
	return false
}

// ToggleFavorite adds target to favorites if not present, or removes it if already present.
// It returns the updated slice and true if the track is now a favorite.
func ToggleFavorite(favorites []Track, target Track) ([]Track, bool) {
	for i, f := range favorites {
		if TracksMatch(f, target) {
			updated := make([]Track, 0, len(favorites)-1)
			updated = append(updated, favorites[:i]...)
			updated = append(updated, favorites[i+1:]...)
			return updated, false
		}
	}
	updated := make([]Track, len(favorites), len(favorites)+1)
	copy(updated, favorites)
	updated = append(updated, target)
	return updated, true
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

func isMountSegment(seg string) bool {
	s := strings.ToLower(seg)
	if s == "" || s == "." || s == "volumes" || s == "music" || s == "mnt" || s == "media" || s == "gvfs" || s == "nasa" || s == "nasa.local" {
		return true
	}
	return strings.Contains(s, "smb-share:") || strings.Contains(s, "nfs:") || strings.Contains(s, "nasa.local")
}

func deriveArtistAlbum(s Song) (string, string) {
	artist := strings.TrimSpace(s.Artist)
	album := strings.TrimSpace(s.Album)

	// Derive from path (.../Artist/Album/Track.ext) if missing or default placeholder
	cleaned := filepath.ToSlash(filepath.Clean(s.Path))
	parts := strings.Split(cleaned, "/")
	if (artist == "" || artist == "Unknown Artist") && len(parts) >= 3 {
		if candidate := parts[len(parts)-3]; !isMountSegment(candidate) {
			artist = candidate
		}
	}
	if (album == "" || album == "Unknown Album") && len(parts) >= 2 {
		if candidate := parts[len(parts)-2]; !isMountSegment(candidate) {
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

// Remove deletes the item at idx from the queue and adjusts CurrentIndex.
// It returns true if the removed track was the currently playing track.
func (q *Queue) Remove(idx int) (removedCurrent bool) {
	if idx < 0 || idx >= len(q.Items) {
		return false
	}
	removedCurrent = idx == q.CurrentIndex
	q.Items = append(q.Items[:idx], q.Items[idx+1:]...)
	if len(q.Items) == 0 {
		q.CurrentIndex = -1
		return removedCurrent
	}
	if idx < q.CurrentIndex {
		q.CurrentIndex--
	} else if q.CurrentIndex >= len(q.Items) {
		q.CurrentIndex = len(q.Items) - 1
	}
	return removedCurrent
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
			if TracksMatch(current, item) {
				q.CurrentIndex = i
				return
			}
		}
	} else {
		q.CurrentIndex = 0
	}
}

// TracksMatch reports whether two Tracks refer to the same local file or YouTube video.
func TracksMatch(a, b Track) bool {
	if a.IsLocal && b.IsLocal {
		return a.Source == b.Source
	}
	if a.VideoID != "" && b.VideoID != "" {
		return a.VideoID == b.VideoID
	}
	return a.Source == b.Source
}

func tracksMatch(a, b Track) bool {
	return TracksMatch(a, b)
}
