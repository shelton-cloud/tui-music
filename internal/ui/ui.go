package ui

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"tui-music/internal/library"
	"tui-music/internal/player"
	"tui-music/internal/youtube"
)

// PlayerBackend abstracts the mpv IPC client so the UI can be tested without audio hardware.
type PlayerBackend interface {
	Play(source string) error
	TogglePause() error
	Stop() error
	Seek(seconds float64) error
	Duration() float64
	TimePos() float64
	Paused() bool
	Title() string
	Artist() string
	PeakLevel() float64
	Updates() <-chan player.State
	Close() error
}

// VolumeController is an optional extension implemented by player.Client for volume control.
type VolumeController interface {
	Volume() float64
	SetVolume(vol float64) error
	AdjustVolume(delta float64) error
}

// SearchFunc resolves a YouTube Music search query into results.
type SearchFunc func(ctx context.Context, query string) ([]youtube.Result, error)

// RadioFunc resolves a seed YouTube VideoID into an "Up Next" radio track list.
type RadioFunc func(ctx context.Context, videoID string, limit int) ([]youtube.Result, error)

// LyricsFunc resolves track title and artist into synced or plain lyric lines.
type LyricsFunc func(ctx context.Context, title, artist string) ([]youtube.LyricLine, error)

type paneFocus int

const (
	focusQueue paneFocus = iota
	focusTree
)

type leftPaneMode int

const (
	modeMixtape leftPaneMode = iota
	modeNASTree
)

var (
	cassetteFrame = []string{
		" ╔══════════════════╗ ",
		" ║ 󰓠  NASA TUI      ║ ",
		" ╟──┬────────────┬──╢ ",
		" ║  │  %s      %s  │  ║ ",
		" ║  ╰────────────╯  ║ ",
		" ╚══════╗    ╔══════╝ ",
		"        ╚════╝        ",
	}
	reelFrames = []string{"◐", "◓", "◑", "◒"}
)

// Retro CRT palette (Dracula + Phosphor Green & Amber CRT accents)
var (
	colorBg       = lipgloss.Color("#282a36")
	colorDarkBg   = lipgloss.Color("#1e1f29")
	colorHeaderBg = lipgloss.Color("#343746")
	colorPhosphor = lipgloss.Color("#50fa7b")
	colorAmber    = lipgloss.Color("#ffb86c")
	colorPink     = lipgloss.Color("#ff79c6")
	colorCyan     = lipgloss.Color("#8be9fd")
	colorPurple   = lipgloss.Color("#bd93f9")
	colorMuted    = lipgloss.Color("#6272a4")
	colorAlertRed = lipgloss.Color("#ff5555")

	breadcrumbStyle = lipgloss.NewStyle().
			Background(colorHeaderBg).
			Foreground(colorCyan).
			Italic(true).
			Padding(0, 1)

	paneHeaderActiveStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAmber).
				Background(colorHeaderBg).
				Padding(0, 1)

	paneHeaderInactiveStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorPink).
				Background(colorHeaderBg).
				Padding(0, 1)

	statusBarStyle = lipgloss.NewStyle().
			Bold(true).
			Background(colorPurple).
			Foreground(colorBg).
			Padding(0, 1)

	cassetteStyle = lipgloss.NewStyle().
			Foreground(colorPink)

	reelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAmber)

	waveStyle = lipgloss.NewStyle().
			Foreground(colorPhosphor)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorCyan)

	artistStyle = lipgloss.NewStyle().
			Foreground(colorPurple)

	albumStyle = lipgloss.NewStyle().
			Foreground(colorAmber)

	currentTrackStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorPhosphor)

	cursorRowStyle = lipgloss.NewStyle().
			Background(colorHeaderBg).
			Foreground(colorCyan)
)

type tickMsg time.Time
type playerStateMsg player.State
type searchResultsMsg struct {
	results []youtube.Result
	err     error
}
type radioResultsMsg struct {
	seedVideoID string
	results     []youtube.Result
	err         error
}
type lyricsResultMsg struct {
	title  string
	artist string
	lines  []youtube.LyricLine
	err    error
}

// Model is the Bubble Tea state model for tui-music.
type Model struct {
	player     PlayerBackend
	searchFn   SearchFunc
	radioFn    RadioFunc
	lyricsFn   LyricsFunc
	allSongs   []library.Song
	tree       *library.Tree
	queue      *library.Queue
	queueIndex int
	focus      paneFocus
	leftMode   leftPaneMode
	nasMounted bool

	favorites     []library.Track
	favoritesPath string
	mixtapeCursor int

	autoRadio     bool
	radioFetching bool
	lastRadioSeed string

	showLyrics    bool
	lyricsLoading bool
	lyricsKey     string
	lyricsLines   []youtube.LyricLine

	width      int
	height     int
	breadcrumb string

	isPlaying       bool
	isTransitioning bool
	trackTitle      string
	trackArtist     string
	trackAlbum      string
	timePos         float64
	duration        float64
	volume          float64
	peakLevel       float64

	reelIndex   int
	tickCounter int
	wavePhase   float64
	waveHistory []float64

	// Search modal state
	searchOpen    bool
	searchInput   textinput.Model
	searchStatus  string
	searchResults []youtube.Result
	searchCursor  int

	rng *rand.Rand
}

// NewModel constructs the Bubble Tea Model with the provided player backend and song index.
func NewModel(p PlayerBackend, songs []library.Song, searchFn SearchFunc) *Model {
	return NewModelWithNAS(p, songs, searchFn, len(songs) > 0)
}

// NewModelWithNAS constructs the Bubble Tea Model with explicit NAS availability so
// startup does not queue, stat, or play unmounted NAS tracks when nasMounted is false.
func NewModelWithNAS(p PlayerBackend, songs []library.Song, searchFn SearchFunc, nasMounted bool) *Model {
	if searchFn == nil {
		searchFn = youtube.Search
	}
	ti := textinput.New()
	ti.Placeholder = "Search YouTube Music for a song, artist, or mix..."
	ti.CharLimit = 120
	ti.SetWidth(50)

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	q := library.NewQueue()
	if nasMounted && len(songs) > 0 {
		q.LoadInitialFromLibrary(songs, rng)
	}

	favPath := library.DefaultFavoritesPath()
	favs, _ := library.LoadFavorites(favPath)

	leftMode := modeNASTree
	if !nasMounted || len(songs) == 0 {
		leftMode = modeMixtape
	}

	vol := 100.0
	if vc, ok := p.(VolumeController); ok {
		if v := vc.Volume(); v > 0 {
			vol = v
		}
	}

	m := &Model{
		player:        p,
		searchFn:      searchFn,
		radioFn:       youtube.FetchRadio,
		lyricsFn:      youtube.FetchLyrics,
		allSongs:      songs,
		tree:          library.NewTree(songs),
		queue:         q,
		focus:         focusQueue,
		leftMode:      leftMode,
		nasMounted:    nasMounted,
		favorites:     favs,
		favoritesPath: favPath,
		autoRadio:     true,
		width:         80,
		height:        24,
		breadcrumb:    "󰎆 NASA TUI",
		trackTitle:    "No Track Playing",
		trackArtist:   "",
		trackAlbum:    "",
		volume:        vol,
		waveHistory:   make([]float64, 100),
		searchInput:   ti,
		rng:           rng,
	}

	if !nasMounted {
		m.ConfigurePrimaryYTMusic(false)
		return m
	}

	if len(m.queue.Items) > 0 {
		m.playTrack(0)
	} else if len(m.favorites) > 0 {
		for _, fav := range m.favorites {
			m.queue.Add(fav, false)
		}
		m.playTrack(0)
	}
	return m
}

// ConfigurePrimaryYTMusic configures the model at startup when YouTube Music is primary
// and checks whether the local NAS mount is actually reachable.
func (m *Model) ConfigurePrimaryYTMusic(nasMounted bool) {
	m.nasMounted = nasMounted
	if !nasMounted {
		m.leftMode = modeMixtape
		hadItems := len(m.queue.Items) > 0
		// Clear unmounted NAS tracks from the startup queue so we don't queue unplayable paths
		m.queue.Items = nil
		m.queue.CurrentIndex = -1
		m.queueIndex = 0
		m.isPlaying = false
		if hadItems && m.player != nil {
			_ = m.player.Stop()
		}
		if len(m.favorites) > 0 {
			for _, fav := range m.favorites {
				if !fav.IsLocal || fileExists(fav.Source) {
					m.queue.Add(fav, false)
				}
			}
		}
		if len(m.queue.Items) > 0 {
			m.playTrack(0)
		} else {
			m.trackTitle = "Press [/] to Search YouTube Music"
			m.trackArtist = "YouTube Music Primary • NAS Offline"
			m.trackAlbum = "Ready"
			m.breadcrumb = "󰎆 YT MUSIC PRIMARY (NAS OFFLINE — PRESS [/] TO SEARCH)"
		}
	}
}

func fileExists(p string) bool {
	return library.PathExistsFast(p)
}

// SetRadioFunc overrides the YouTube Music Auto-Radio fetcher (useful for testing).
func (m *Model) SetRadioFunc(fn RadioFunc) {
	m.radioFn = fn
}

// SetLyricsFunc overrides the lyrics fetcher (useful for testing).
func (m *Model) SetLyricsFunc(fn LyricsFunc) {
	m.lyricsFn = fn
}

// SetFavoritesPath overrides the file path used to persist favorites (useful for testing).
func (m *Model) SetFavoritesPath(path string) {
	m.favoritesPath = path
	if favs, err := library.LoadFavorites(path); err == nil {
		m.favorites = favs
	}
}

// Init starts the animation ticker and subscribes to non-blocking mpv state updates.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		waitForPlayerState(m.player),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func waitForPlayerState(p PlayerBackend) tea.Cmd {
	if p == nil {
		return nil
	}
	ch := p.Updates()
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		st, ok := <-ch
		if !ok {
			return nil
		}
		return playerStateMsg(st)
	}
}

func (m *Model) fetchRadioCmd(videoID string) tea.Cmd {
	if m.radioFn == nil || videoID == "" || m.radioFetching || m.lastRadioSeed == videoID {
		return nil
	}
	m.radioFetching = true
	m.lastRadioSeed = videoID
	fn := m.radioFn
	return func() tea.Msg {
		res, err := fn(context.Background(), videoID, 15)
		return radioResultsMsg{seedVideoID: videoID, results: res, err: err}
	}
}

func (m *Model) fetchLyricsCmd(title, artist string) tea.Cmd {
	if m.lyricsFn == nil || title == "" || title == "No Track Playing" {
		return nil
	}
	key := strings.ToLower(strings.TrimSpace(title) + "|" + strings.TrimSpace(artist))
	if m.lyricsKey == key && (m.lyricsLoading || len(m.lyricsLines) > 0) {
		return nil
	}
	m.lyricsKey = key
	m.lyricsLoading = true
	m.lyricsLines = nil
	fn := m.lyricsFn
	return func() tea.Msg {
		lines, err := fn(context.Background(), title, artist)
		return lyricsResultMsg{title: title, artist: artist, lines: lines, err: err}
	}
}

// Update handles keyboard input, terminal resizes, animation ticks, and mpv IPC updates.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		m.tickCounter++
		// Advance cassette reels every ~240ms (3 ticks * 80ms) while playing
		if m.isPlaying && m.tickCounter%3 == 0 {
			m.reelIndex = (m.reelIndex + 1) % len(reelFrames)
		}
		m.stepWaveVisualizer()
		return m, tickCmd()

	case playerStateMsg:
		cmd := m.applyPlayerState(player.State(msg))
		return m, tea.Batch(waitForPlayerState(m.player), cmd)

	case searchResultsMsg:
		if msg.err != nil {
			m.searchStatus = fmt.Sprintf("Search error: %v", msg.err)
		} else {
			m.searchResults = msg.results
			m.searchCursor = 0
			m.searchStatus = fmt.Sprintf("Found %d results — [ENTER] Play Now | [TAB] Queue Next", len(msg.results))
		}
		return m, nil

	case radioResultsMsg:
		m.radioFetching = false
		if msg.err == nil && len(msg.results) > 0 {
			added := 0
			for _, r := range msg.results {
				track := r.ToTrack()
				if !m.isTrackInQueue(track) {
					m.queue.Add(track, false)
					added++
				}
			}
			if added > 0 {
				m.breadcrumb = fmt.Sprintf("STREAMING > YT RADIO > +%d UP NEXT TRACKS QUEUED", added)
			}
		}
		return m, nil

	case lyricsResultMsg:
		m.lyricsLoading = false
		if msg.err == nil {
			m.lyricsLines = msg.lines
		}
		return m, nil

	case tea.KeyPressMsg:
		if m.searchOpen {
			return m.updateSearchModal(msg)
		}
		return m.updateMainView(msg)
	}

	if m.searchOpen {
		var cmd tea.Cmd
		prevVal := m.searchInput.Value()
		m.searchInput, cmd = m.searchInput.Update(msg)
		if m.searchInput.Value() != prevVal {
			m.searchResults = nil
		}
		return m, cmd
	}

	return m, nil
}

func (m *Model) isTrackInQueue(t library.Track) bool {
	for _, item := range m.queue.Items {
		if library.TracksMatch(item, t) {
			return true
		}
	}
	return false
}

func (m *Model) updateSearchModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchOpen = false
		m.searchInput.Blur()
		return m, nil

	case "up":
		if m.searchCursor > 0 {
			m.searchCursor--
		}
		return m, nil

	case "down":
		if m.searchCursor+1 < len(m.searchResults) {
			m.searchCursor++
		}
		return m, nil

	case "tab", "ctrl+a":
		// Append highlighted result to queue without closing search modal or interrupting playback
		if len(m.searchResults) > 0 && m.searchCursor >= 0 && m.searchCursor < len(m.searchResults) && m.searchStatus != "Searching..." {
			selected := m.searchResults[m.searchCursor]
			track := selected.ToTrack()
			m.addTrackToQueue(track, false)
			m.searchStatus = fmt.Sprintf("Queued: %s - %s", track.Title, track.Artist)
			if m.autoRadio && track.VideoID != "" {
				return m, m.fetchRadioCmd(track.VideoID)
			}
		}
		return m, nil

	case "enter":
		query := strings.TrimSpace(m.searchInput.Value())
		if len(m.searchResults) > 0 && m.searchCursor >= 0 && m.searchCursor < len(m.searchResults) && m.searchStatus != "Searching..." {
			selected := m.searchResults[m.searchCursor]
			m.searchOpen = false
			m.searchInput.Blur()
			track := selected.ToTrack()
			cmd := m.playOrQueueImmediate(track)
			return m, cmd
		}
		if query != "" {
			m.searchStatus = "Searching..."
			m.searchResults = nil
			searchFn := m.searchFn
			return m, func() tea.Msg {
				res, err := searchFn(context.Background(), query)
				return searchResultsMsg{results: res, err: err}
			}
		}
		return m, nil
	}

	// If user types a new character, clear old results so pressing Enter triggers a new search
	var cmd tea.Cmd
	prevVal := m.searchInput.Value()
	m.searchInput, cmd = m.searchInput.Update(msg)
	if m.searchInput.Value() != prevVal {
		m.searchResults = nil
	}
	return m, cmd
}

func (m *Model) playOrQueueImmediate(track library.Track) tea.Cmd {
	m.queue.Add(track, false)
	idx := len(m.queue.Items) - 1
	m.playTrack(idx)
	var cmds []tea.Cmd
	if m.autoRadio && track.VideoID != "" {
		cmds = append(cmds, m.fetchRadioCmd(track.VideoID))
	}
	if m.showLyrics {
		cmds = append(cmds, m.fetchLyricsCmd(track.Title, track.Artist))
	}
	return tea.Batch(cmds...)
}

func (m *Model) updateMainView(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		if m.player != nil {
			_ = m.player.Close()
		}
		return m, tea.Quit

	case "space":
		m.actionTogglePause()
		return m, nil

	case "n":
		return m, m.actionNextTrack()

	case "p":
		return m, m.actionPrevTrack()

	case "s":
		m.actionShuffle()
		return m, nil

	case "r":
		m.autoRadio = !m.autoRadio
		state := "OFF"
		if m.autoRadio {
			state = "ON"
		}
		m.breadcrumb = fmt.Sprintf("󰎆 YOUTUBE AUTO-RADIO: %s", state)
		if m.autoRadio {
			if cur, ok := m.queue.Current(); ok && cur.VideoID != "" {
				return m, m.fetchRadioCmd(cur.VideoID)
			}
		}
		return m, nil

	case "f":
		m.actionToggleFavorite()
		return m, nil

	case "l":
		m.showLyrics = !m.showLyrics
		if m.showLyrics {
			return m, m.fetchLyricsCmd(m.trackTitle, m.trackArtist)
		}
		return m, nil

	case "d", "x", "backspace", "delete":
		if m.focus == focusQueue && len(m.queue.Items) > 0 {
			removedCurrent := m.queue.Remove(m.queueIndex)
			if m.queueIndex >= len(m.queue.Items) && m.queueIndex > 0 {
				m.queueIndex = len(m.queue.Items) - 1
			}
			if removedCurrent {
				if len(m.queue.Items) > 0 && m.queue.CurrentIndex >= 0 {
					m.playTrack(m.queue.CurrentIndex)
				} else if m.player != nil {
					_ = m.player.Stop()
					m.isPlaying = false
				}
			}
		}
		return m, nil

	case "-", "_":
		m.actionAdjustVolume(-5)
		return m, nil

	case "+", "=":
		m.actionAdjustVolume(5)
		return m, nil

	case ",", "[":
		if m.player != nil {
			_ = m.player.Seek(-5)
		}
		return m, nil

	case ".", "]":
		if m.player != nil {
			_ = m.player.Seek(5)
		}
		return m, nil

	case "1":
		m.leftMode = modeMixtape
		m.focus = focusTree
		return m, nil

	case "2":
		m.leftMode = modeNASTree
		m.focus = focusTree
		return m, nil

	case "m":
		if m.leftMode == modeMixtape {
			m.leftMode = modeNASTree
		} else {
			m.leftMode = modeMixtape
		}
		return m, nil

	case "/":
		m.searchOpen = true
		m.searchStatus = "Press ENTER to search YouTube Music, then ENTER to play (or TAB to queue)"
		m.searchResults = nil
		m.searchCursor = 0
		m.searchInput.SetValue("")
		cmd := m.searchInput.Focus()
		return m, cmd

	case "tab", "left", "right":
		if m.focus == focusQueue {
			m.focus = focusTree
		} else {
			m.focus = focusQueue
		}
		return m, nil

	case "up":
		if m.focus == focusTree {
			if m.leftMode == modeMixtape {
				if m.mixtapeCursor > 0 {
					m.mixtapeCursor--
				}
			} else {
				m.tree.MoveUp()
			}
		} else if m.queueIndex > 0 {
			m.queueIndex--
		}
		return m, nil

	case "down":
		if m.focus == focusTree {
			if m.leftMode == modeMixtape {
				if m.mixtapeCursor+1 < len(m.favorites) {
					m.mixtapeCursor++
				}
			} else {
				m.tree.MoveDown()
			}
		} else if m.queueIndex+1 < len(m.queue.Items) {
			m.queueIndex++
		}
		return m, nil

	case "enter":
		if m.focus == focusTree {
			if m.leftMode == modeMixtape {
				if m.mixtapeCursor >= 0 && m.mixtapeCursor < len(m.favorites) {
					fav := m.favorites[m.mixtapeCursor]
					return m, m.playOrQueueImmediate(fav)
				}
				return m, nil
			}
			if song := m.tree.Activate(); song != nil {
				track := library.TrackFromSong(*song)
				m.addTrackToQueue(track, true)
				dirName := strings.ToUpper(filepath.Base(filepath.Dir(song.Path)))
				m.breadcrumb = fmt.Sprintf("NAS > %s > %s", dirName, filepath.Base(song.Path))
			}
		} else {
			m.playTrack(m.queueIndex)
			if m.showLyrics {
				return m, m.fetchLyricsCmd(m.trackTitle, m.trackArtist)
			}
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) actionAdjustVolume(delta float64) {
	if vc, ok := m.player.(VolumeController); ok {
		_ = vc.AdjustVolume(delta)
		m.volume = vc.Volume()
		return
	}
	m.volume = math.Max(0, math.Min(130, m.volume+delta))
}

func (m *Model) actionToggleFavorite() {
	var target library.Track
	var ok bool
	if m.focus == focusQueue && m.queueIndex >= 0 && m.queueIndex < len(m.queue.Items) {
		target = m.queue.Items[m.queueIndex]
		ok = true
	} else if cur, hasCur := m.queue.Current(); hasCur {
		target = cur
		ok = true
	}
	if !ok {
		return
	}
	var added bool
	m.favorites, added = library.ToggleFavorite(m.favorites, target)
	if m.favoritesPath != "" {
		_ = library.SaveFavorites(m.favoritesPath, m.favorites)
	}
	if m.mixtapeCursor >= len(m.favorites) && len(m.favorites) > 0 {
		m.mixtapeCursor = len(m.favorites) - 1
	}
	if added {
		m.breadcrumb = fmt.Sprintf("★ SAVED TO MIXTAPE: %s - %s", target.Title, target.Artist)
	} else {
		m.breadcrumb = fmt.Sprintf("☆ REMOVED FROM MIXTAPE: %s - %s", target.Title, target.Artist)
	}
}

func (m *Model) addTrackToQueue(track library.Track, playNow bool) {
	wasEmpty := m.queue.CurrentIndex == -1
	started := m.queue.Add(track, playNow)
	if started || (playNow && wasEmpty) {
		m.playTrack(len(m.queue.Items) - 1)
	}
}

func (m *Model) playTrack(index int) {
	m.playTrackWithSkips(index, 0, -1)
}

func (m *Model) playTrackWithSkips(index int, skips int, hasLocal int) {
	if index < 0 || index >= len(m.queue.Items) {
		return
	}
	if skips >= len(m.queue.Items) {
		return
	}

	track, ok := m.queue.Select(index)
	if !ok {
		return
	}
	m.queueIndex = index

	// If local file does not exist and we have a live player, skip to next (matching app.py lines 523-526)
	// When running without the NAS mounted, we still update the UI and let mpv attempt playback if desired,
	// unless the file was added as a raw missing path and alternatives exist.
	if track.IsLocal && m.player != nil {
		if !library.PathExistsFast(track.Source) {
			if hasLocal < 0 {
				if m.anyLocalFileExists() {
					hasLocal = 1
				} else {
					hasLocal = 0
				}
			}
			// Check if any file in the queue exists before skipping everything on an unmounted NAS
			if hasLocal == 1 && index+1 < len(m.queue.Items) && skips < 15 {
				m.isTransitioning = false
				m.playTrackWithSkips(index+1, skips+1, hasLocal)
				return
			}
		}
	}

	m.isTransitioning = false
	if m.player != nil {
		_ = m.player.Play(track.Source)
	}

	m.isPlaying = true
	m.trackTitle = track.Title
	m.trackArtist = track.Artist
	m.trackAlbum = track.Album
	m.timePos = 0

	if track.IsLocal {
		m.breadcrumb = fmt.Sprintf("NAS > %s", filepath.Base(track.Source))
	} else {
		m.breadcrumb = fmt.Sprintf("STREAMING > YT MUSIC > %s", strings.ToUpper(track.Title))
	}
}

func (m *Model) anyLocalFileExists() bool {
	checked := 0
	for _, item := range m.queue.Items {
		if !item.IsLocal {
			return true
		}
		if library.PathExistsFast(item.Source) {
			return true
		}
		checked++
		if checked >= 5 {
			break
		}
	}
	return false
}

func (m *Model) actionTogglePause() {
	if m.player != nil {
		_ = m.player.TogglePause()
		m.isPlaying = !m.player.Paused()
	} else {
		m.isPlaying = !m.isPlaying
	}
}

func (m *Model) actionNextTrack() tea.Cmd {
	if m.isTransitioning {
		return nil
	}
	if m.queue.CurrentIndex+1 < len(m.queue.Items) {
		m.isTransitioning = true
		m.playTrack(m.queue.CurrentIndex + 1)
		var cmds []tea.Cmd
		if m.showLyrics {
			cmds = append(cmds, m.fetchLyricsCmd(m.trackTitle, m.trackArtist))
		}
		if m.autoRadio && len(m.queue.Items)-m.queue.CurrentIndex <= 3 {
			if cur, ok := m.queue.Current(); ok && cur.VideoID != "" {
				cmds = append(cmds, m.fetchRadioCmd(cur.VideoID))
			}
		}
		return tea.Batch(cmds...)
	}
	return nil
}

func (m *Model) actionPrevTrack() tea.Cmd {
	if m.queue.CurrentIndex-1 >= 0 {
		m.playTrack(m.queue.CurrentIndex - 1)
		if m.showLyrics {
			return m.fetchLyricsCmd(m.trackTitle, m.trackArtist)
		}
	}
	return nil
}

func (m *Model) actionShuffle() {
	if len(m.queue.Items) == 0 {
		return
	}
	hadCurrent := m.queue.CurrentIndex != -1
	m.queue.Shuffle(m.rng)
	m.queueIndex = m.queue.CurrentIndex
	if !hadCurrent && len(m.queue.Items) > 0 {
		m.playTrack(0)
	}
}

func (m *Model) applyPlayerState(st player.State) tea.Cmd {
	m.timePos = st.TimePos
	if st.Duration > 0 {
		m.duration = st.Duration
	}
	if st.Volume > 0 {
		m.volume = st.Volume
	}
	m.isPlaying = !st.Paused
	m.peakLevel = st.PeakLevel

	cur, hasCur := m.queue.Current()
	isYTStream := hasCur && !cur.IsLocal

	// For YouTube streams, preserve the clean track metadata from YT Music rather than
	// overwriting with mpv's raw stream title or "Unknown Artist".
	if !isYTStream || m.trackTitle == "" || m.trackTitle == "No Track Playing" {
		if st.Title != "" && st.Title != "No Track Playing" {
			m.trackTitle = st.Title
		}
	}
	if !isYTStream || m.trackArtist == "" || m.trackArtist == "Unknown" || m.trackArtist == "Unknown Artist" {
		if st.Artist != "" && st.Artist != "Unknown Artist" {
			m.trackArtist = st.Artist
		}
	}
	if hasCur && m.trackAlbum == "" {
		m.trackAlbum = cur.Album
	}

	// Auto-next on EOF or within 0.8s of duration (matching app.py line 590)
	if !m.isTransitioning {
		if st.EOF || (m.duration > 0 && m.timePos >= m.duration-0.8) {
			return m.actionNextTrack()
		}
	}
	return nil
}

func (m *Model) stepWaveVisualizer() {
	targetLen := 50
	if m.width > 25 {
		targetLen = 50
	}
	if len(m.waveHistory) < targetLen {
		pad := make([]float64, targetLen-len(m.waveHistory))
		m.waveHistory = append(pad, m.waveHistory...)
	} else if len(m.waveHistory) > targetLen {
		m.waveHistory = m.waveHistory[len(m.waveHistory)-targetLen:]
	}

	var newVal float64
	if m.isPlaying {
		rawPeak := m.peakLevel
		if rawPeak > 0 {
			// Match app.py line 138: min(1.0, (raw_peak ** 0.3) * 1.8)
			newVal = math.Min(1.0, math.Pow(rawPeak, 0.3)*1.8)
			newVal = math.Max(0.05, newVal+(m.rng.Float64()*0.04-0.02))
		} else {
			// Determinate multi-harmonic wave when mpv IPC does not emit af-metadata
			m.wavePhase += 0.35
			wave := 0.50 + 0.30*math.Sin(m.wavePhase) + 0.15*math.Cos(m.wavePhase*2.3)
			newVal = math.Max(0.08, math.Min(0.95, wave))
		}
	} else {
		// Match app.py lines 141-142: gentle idle sine wave when paused
		m.wavePhase += 0.1
		newVal = 0.05 + 0.05*math.Sin(m.wavePhase)
	}

	if len(m.waveHistory) > 0 {
		m.waveHistory = append(m.waveHistory[1:], newVal)
	}
}

// RenderBrailleWave renders the Braille dot-matrix wave visualiser matching WaveVisualizer in app.py.
func RenderBrailleWave(history []float64, width, height int) string {
	if width < 5 || height < 1 || len(history) < 2 {
		return ""
	}
	type pair struct{ h1, h2 float64 }
	var dotMatrix []pair
	maxCols := width * 2
	if len(history) < maxCols {
		maxCols = len(history)
	}
	for c := 0; c+1 < maxCols; c += 2 {
		dotMatrix = append(dotMatrix, pair{history[c], history[c+1]})
	}

	rows := make([]string, 0, height)
	for r := 0; r < height; r++ {
		bThresh := float64(height-r-1) / float64(height)
		tThresh := float64(height-r) / float64(height)
		var sb strings.Builder
		for _, p := range dotMatrix {
			code := brailleCode(p.h1, p.h2, bThresh, tThresh)
			if code != 0 {
				sb.WriteRune(rune(0x2800 + code))
			} else {
				sb.WriteByte(' ')
			}
		}
		rows = append(rows, sb.String())
	}
	return strings.Join(rows, "\n")
}

func brailleCode(h1, h2, bThresh, tThresh float64) int {
	getDots := func(val float64) [4]bool {
		rel := (val - bThresh) / (tThresh - bThresh)
		if rel <= 0 {
			return [4]bool{false, false, false, false}
		}
		if rel > 0.75 {
			return [4]bool{true, true, true, true}
		}
		if rel > 0.5 {
			return [4]bool{false, true, true, true}
		}
		if rel > 0.25 {
			return [4]bool{false, false, true, true}
		}
		return [4]bool{false, false, false, true}
	}

	d1 := getDots(h1)
	d2 := getDots(h2)
	code := 0
	if d1[0] {
		code |= 0x01
	}
	if d1[1] {
		code |= 0x02
	}
	if d1[2] {
		code |= 0x04
	}
	if d1[3] {
		code |= 0x40
	}
	if d2[0] {
		code |= 0x08
	}
	if d2[1] {
		code |= 0x10
	}
	if d2[2] {
		code |= 0x20
	}
	if d2[3] {
		code |= 0x80
	}
	return code
}

// View renders the TUI screen.
func (m *Model) View() tea.View {
	if m.width < 60 || m.height < 15 {
		gateStyle := lipgloss.NewStyle().
			Width(m.width).
			Height(m.height).
			Align(lipgloss.Center, lipgloss.Center).
			Bold(true).
			Foreground(colorAlertRed).
			Background(colorBg)
		view := tea.NewView(gateStyle.Render("Terminal too small! Please resize your pane.\n(Min 60x15 required)"))
		view.AltScreen = true
		return view
	}

	if m.searchOpen {
		view := tea.NewView(m.renderSearchModal())
		view.AltScreen = true
		return view
	}

	breadcrumb := breadcrumbStyle.
		Width(m.width).
		MaxHeight(1).
		Render(truncateStr(m.breadcrumb, m.width-2))

	// Exact vertical budget so View() is always m.height rows (Bubble Tea v2 clips > m.height).
	// NowPlaying renders 7 cassette rows (+ 2 wave rows when height >= 19) + 2 double-border rows = 11 (or 9) rows.
	availHeight := m.height - 2 // 1 row breadcrumb + 1 row control bar
	nowPlayingHeight := 11
	if m.height < 19 {
		nowPlayingHeight = 9
	}
	topHeight := availHeight - nowPlayingHeight
	if topHeight < 4 {
		topHeight = 4
	}

	topPane := m.renderTopPanes(m.width, topHeight)
	nowPlaying := m.renderNowPlaying(m.width, nowPlayingHeight)
	statusBar := m.renderControlBar(m.width)

	view := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, breadcrumb, topPane, nowPlaying, statusBar))
	view.AltScreen = true
	return view
}

func (m *Model) renderControlBar(totalWidth int) string {
	type controlBtn struct {
		key    string
		label  string
		active bool
		accent bool
	}

	playLabel := "Play"
	if m.isPlaying {
		playLabel = "Pause"
	}

	quitBtn := controlBtn{key: "Q", label: "Quit", accent: true}
	buttons := []controlBtn{
		{key: "/", label: "Search"},
		{key: "SPACE", label: playLabel},
		{key: "N/P", label: "Track"},
		{key: "R", label: "Radio", active: m.autoRadio},
		{key: "F", label: "Fav"},
		{key: "L", label: "Lyrics", active: m.showLyrics},
		{key: "S", label: "Shuffle"},
		{key: "-/+", label: "Vol"},
		{key: "1/2", label: "Source"},
	}

	keyBase := lipgloss.NewStyle().Bold(true).Foreground(colorDarkBg).Background(colorPurple)
	keyActive := lipgloss.NewStyle().Bold(true).Foreground(colorDarkBg).Background(colorPhosphor)
	keyAccent := lipgloss.NewStyle().Bold(true).Foreground(colorDarkBg).Background(colorPink)
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(colorCyan).Background(colorHeaderBg)
	sepStyle := lipgloss.NewStyle().Background(colorHeaderBg)

	renderBtn := func(b controlBtn) (string, int) {
		ks := keyBase
		if b.active {
			ks = keyActive
		} else if b.accent {
			ks = keyAccent
		}
		keyText := "[" + b.key + "]"
		lblText := " " + b.label + " "
		rendered := ks.Render(keyText) + labelStyle.Render(lblText)
		return rendered, lipgloss.Width(keyText) + lipgloss.Width(lblText)
	}

	quitRendered, quitWidth := renderBtn(quitBtn)
	sep := sepStyle.Render(" ")

	var parts []string
	used := 1 + quitWidth // leading space + quit button
	for _, b := range buttons {
		rendered, w := renderBtn(b)
		needed := w + 1 // button + 1 separator space
		if used+needed > totalWidth-1 {
			break
		}
		parts = append(parts, rendered)
		used += needed
	}
	parts = append(parts, quitRendered)

	row := sep + strings.Join(parts, sep)
	rowWidth := lipgloss.Width(row)
	if rowWidth < totalWidth {
		row += sepStyle.Render(strings.Repeat(" ", totalWidth-rowWidth))
	}
	return lipgloss.NewStyle().MaxWidth(totalWidth).MaxHeight(1).Render(row)
}

func (m *Model) renderTopPanes(totalWidth, totalHeight int) string {
	leftWidth := totalWidth / 2
	rightWidth := totalWidth - leftWidth
	innerHeight := totalHeight - 2
	if innerHeight < 2 {
		innerHeight = 2
	}

	treeBorderColor := colorMuted
	queueBorderColor := colorPurple
	treeHdrStyle := paneHeaderInactiveStyle
	queueHdrStyle := paneHeaderInactiveStyle
	if m.focus == focusTree {
		treeBorderColor = colorPhosphor
		treeHdrStyle = paneHeaderActiveStyle
	} else {
		queueBorderColor = colorPhosphor
		queueHdrStyle = paneHeaderActiveStyle
	}

	// Lip Gloss v2 uses border-box sizing: Width and Height include the 2 border cells.
	treeBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(treeBorderColor).
		Width(leftWidth).
		Height(totalHeight).
		MaxHeight(totalHeight)

	queueBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(queueBorderColor).
		Width(rightWidth).
		Height(totalHeight).
		MaxHeight(totalHeight)

	leftInnerWidth := max(4, leftWidth-2)
	rightInnerWidth := max(4, rightWidth-2)
	listRows := max(1, innerHeight-1)

	var leftContent string
	if m.leftMode == modeMixtape {
		leftContent = m.renderMixtapeList(leftInnerWidth, listRows, treeHdrStyle)
	} else {
		leftContent = m.renderTreeList(leftInnerWidth, listRows, treeHdrStyle)
	}
	queueContent := m.renderQueueList(rightInnerWidth, listRows, queueHdrStyle)

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		treeBoxStyle.Render(leftContent),
		queueBoxStyle.Render(queueContent),
	)
}

func (m *Model) renderMixtapeList(width, listHeight int, hdrStyle lipgloss.Style) string {
	nasTag := "NAS: OFFLINE"
	if m.nasMounted {
		nasTag = "NAS: READY [2]"
	}
	header := hdrStyle.Width(width).MaxHeight(1).Render(truncateStr(fmt.Sprintf("★ YT MUSIC & MIXTAPE (%s)", nasTag), width-2))
	if listHeight < 1 {
		return header
	}
	if len(m.favorites) == 0 {
		hint := []string{
			header,
			lipgloss.NewStyle().Foreground(colorCyan).Render(truncateStr(" 󰍉 Press [/] to search YouTube Music", width)),
			lipgloss.NewStyle().Foreground(colorMuted).Render(truncateStr(" ★ Press [F] on any track to dub it to your Mixtape", width)),
			lipgloss.NewStyle().Foreground(colorMuted).Render(truncateStr(" 󰒝 Auto-Radio [R] automatically queues related YT songs", width)),
			lipgloss.NewStyle().Foreground(colorMuted).Render(truncateStr(" 󰙅 Press [2] or [M] to switch to NAS Library Tree", width)),
		}
		if len(hint) > listHeight+1 {
			hint = hint[:listHeight+1]
		}
		return strings.Join(hint, "\n")
	}

	start, end := visibleWindow(m.mixtapeCursor, len(m.favorites), listHeight)
	rows := make([]string, 0, listHeight+1)
	rows = append(rows, header)

	for i := start; i < end; i++ {
		fav := m.favorites[i]
		badge := "[YT]"
		if fav.IsLocal {
			badge = "[NAS]"
		}
		line := truncateStr(fmt.Sprintf(" ★ %s %s - %s", badge, fav.Title, fav.Artist), width)
		if i == m.mixtapeCursor && m.focus == focusTree {
			line = cursorRowStyle.Width(width).MaxHeight(1).Render(line)
		}
		rows = append(rows, line)
	}
	return strings.Join(rows, "\n")
}

func (m *Model) renderTreeList(width, listHeight int, hdrStyle lipgloss.Style) string {
	mountBadge := ""
	if !m.nasMounted {
		mountBadge = " [UNMOUNTED]"
	}
	header := hdrStyle.Width(width).MaxHeight(1).Render(truncateStr("󰙅 LIBRARY (Artist → Album → Track)"+mountBadge, width-2))
	if listHeight < 1 {
		return header
	}
	if len(m.tree.Visible) == 0 {
		emptyMsg := truncateStr(" (No songs loaded in songs.json — press [/] for YT Music)", width)
		return header + "\n" + lipgloss.NewStyle().Foreground(colorMuted).Render(emptyMsg)
	}

	start, end := visibleWindow(m.tree.Cursor, len(m.tree.Visible), listHeight)
	rows := make([]string, 0, listHeight+1)
	rows = append(rows, header)

	for i := start; i < end; i++ {
		flat := m.tree.Visible[i]
		indent := strings.Repeat("  ", flat.Depth)
		var icon string
		switch flat.Node.Kind {
		case library.NodeArtist, library.NodeAlbum:
			if flat.Node.Expanded {
				icon = "▾ 📁 "
			} else {
				icon = "▸ 📁 "
			}
		case library.NodeTrack:
			icon = "  ♪ "
		}
		line := truncateStr(indent+icon+flat.Node.Label, width)
		if i == m.tree.Cursor && m.focus == focusTree {
			line = cursorRowStyle.Width(width).MaxHeight(1).Render(line)
		}
		rows = append(rows, line)
	}
	return strings.Join(rows, "\n")
}

func (m *Model) renderQueueList(width, listHeight int, hdrStyle lipgloss.Style) string {
	radioTag := "RADIO:OFF"
	if m.autoRadio {
		radioTag = "RADIO:ON"
	}
	header := hdrStyle.Width(width).MaxHeight(1).Render(truncateStr(fmt.Sprintf("󰎆 UPCOMING QUEUE (%d • %s)", len(m.queue.Items), radioTag), width-2))
	if listHeight < 1 {
		return header
	}
	if len(m.queue.Items) == 0 {
		emptyMsg := truncateStr(" (Queue is empty — press [/] to search YouTube Music)", width)
		return header + "\n" + lipgloss.NewStyle().Foreground(colorMuted).Render(emptyMsg)
	}

	start, end := visibleWindow(m.queueIndex, len(m.queue.Items), listHeight)
	rows := make([]string, 0, listHeight+1)
	rows = append(rows, header)

	for i := start; i < end; i++ {
		item := m.queue.Items[i]
		prefix := "   "
		if i == m.queue.CurrentIndex {
			prefix = " ▶ "
		}
		favStar := ""
		if library.IsFavorite(m.favorites, item) {
			favStar = "★ "
		}
		srcBadge := "[YT] "
		if item.IsLocal {
			srcBadge = "[NAS] "
		}
		line := truncateStr(fmt.Sprintf("%s%s%s%s - %s", prefix, favStar, srcBadge, item.Title, item.Artist), width)
		switch {
		case i == m.queueIndex && m.focus == focusQueue:
			line = cursorRowStyle.Width(width).MaxHeight(1).Render(line)
		case i == m.queue.CurrentIndex:
			line = currentTrackStyle.Render(line)
		}
		rows = append(rows, line)
	}
	return strings.Join(rows, "\n")
}

func (m *Model) renderNowPlaying(totalWidth, totalHeight int) string {
	reel := reelStyle.Render(reelFrames[m.reelIndex])
	cassetteLines := make([]string, len(cassetteFrame))
	for i, line := range cassetteFrame {
		if i == 3 {
			cassetteLines[i] = fmt.Sprintf(line, reel, reel)
		} else {
			cassetteLines[i] = cassetteStyle.Render(line)
		}
	}
	cassetteBlock := strings.Join(cassetteLines, "\n")
	leftColumn := cassetteBlock
	if totalHeight >= 11 {
		waveBlock := waveStyle.Render(RenderBrailleWave(m.waveHistory, 22, 2))
		leftColumn = lipgloss.JoinVertical(lipgloss.Left, cassetteBlock, waveBlock)
	}

	infoWidth := totalWidth - 28
	if infoWidth < 20 {
		infoWidth = 20
	}

	stateBadge := "[PAUSED]"
	if m.isPlaying {
		stateBadge = "[PLAYING]"
	}
	radioBadge := "[RADIO:OFF]"
	if m.autoRadio {
		radioBadge = "[RADIO:ON]"
	}
	favBadge := ""
	if cur, ok := m.queue.Current(); ok && library.IsFavorite(m.favorites, cur) {
		favBadge = " ★"
	}
	volBadge := fmt.Sprintf("VOL:%d%%", int(math.Round(m.volume)))

	titleLine := titleStyle.Render(truncateStr(m.trackTitle+favBadge, infoWidth))
	artistLine := artistStyle.Render(truncateStr("Artist: "+nonEmpty(m.trackArtist, "Unknown Artist"), infoWidth))
	albumLine := albumStyle.Render(truncateStr(
		fmt.Sprintf("Album:  %s  %s %s [%s]", nonEmpty(m.trackAlbum, "Unknown Album"), stateBadge, radioBadge, volBadge),
		infoWidth,
	))

	var pct float64
	if m.duration > 0 {
		pct = math.Max(0, math.Min(100, (m.timePos/m.duration)*100))
	}
	bar := renderProgressBar(pct, infoWidth-8)
	timeStr := fmt.Sprintf("%s / %s", FormatTime(m.timePos), FormatTime(m.duration))
	timeLine := artistStyle.Width(infoWidth).MaxHeight(1).Align(lipgloss.Right).Render(timeStr)

	middleBlock := ""
	if m.showLyrics {
		middleBlock = m.renderActiveLyrics(infoWidth)
	}

	rightColumn := lipgloss.NewStyle().PaddingLeft(2).Render(
		lipgloss.JoinVertical(lipgloss.Left, titleLine, artistLine, albumLine, middleBlock, bar, timeLine),
	)

	// Lip Gloss v2 uses border-box sizing: Width and Height include DoubleBorder.
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(colorPhosphor).
		Width(totalWidth).
		Height(totalHeight).
		MaxHeight(totalHeight)

	return boxStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, leftColumn, rightColumn))
}

func (m *Model) renderActiveLyrics(width int) string {
	if m.lyricsLoading {
		return lipgloss.NewStyle().Foreground(colorMuted).Italic(true).Render(truncateStr("♪ Fetching synced lyrics...", width))
	}
	if len(m.lyricsLines) == 0 {
		return lipgloss.NewStyle().Foreground(colorMuted).Italic(true).Render(truncateStr("♪ (No synced lyrics found for this track)", width))
	}

	activeIdx := 0
	for i, l := range m.lyricsLines {
		if l.TimeSec >= 0 && l.TimeSec <= m.timePos+0.25 {
			activeIdx = i
		}
	}
	lineText := "♪ " + m.lyricsLines[activeIdx].Text
	return lipgloss.NewStyle().Bold(true).Foreground(colorPhosphor).Render(truncateStr(lineText, width))
}

func renderProgressBar(percent float64, barWidth int) string {
	if barWidth < 10 {
		barWidth = 10
	}
	filled := int(math.Round((percent / 100.0) * float64(barWidth)))
	if filled > barWidth {
		filled = barWidth
	}
	if filled < 0 {
		filled = 0
	}
	empty := barWidth - filled
	barStr := lipgloss.NewStyle().Foreground(colorPhosphor).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(colorMuted).Render(strings.Repeat("░", empty))
	return fmt.Sprintf("%s %5.1f%%", barStr, percent)
}

func (m *Model) renderSearchModal() string {
	modalWidth := (m.width * 80) / 100
	if modalWidth < 50 {
		modalWidth = m.width - 4
	}
	modalHeight := (m.height * 80) / 100
	if modalHeight < 12 {
		modalHeight = 12
	}

	innerWidth := max(20, modalWidth-4)
	resultInnerWidth := max(16, innerWidth-2)

	title := lipgloss.NewStyle().Bold(true).Foreground(colorPink).Render(
		truncateStr("󰍉 SEARCH YOUTUBE MUSIC (PRIMARY STREAMING + AUTO-RADIO)", innerWidth),
	)
	inputView := m.searchInput.View()
	statusView := lipgloss.NewStyle().Foreground(colorAmber).Render(truncateStr(m.searchStatus, innerWidth))

	var resultLines []string
	maxResults := modalHeight - 10
	if maxResults < 3 {
		maxResults = 3
	}
	start, end := visibleWindow(m.searchCursor, len(m.searchResults), maxResults)
	for i := start; i < end; i++ {
		r := m.searchResults[i]
		albumSuffix := ""
		if r.Album != "" && r.Album != "YouTube Music" {
			albumSuffix = " • " + r.Album
		}
		line := truncateStr(fmt.Sprintf("[%s] %s - %s%s", r.Duration, r.Title, r.Artist, albumSuffix), resultInnerWidth-2)
		if i == m.searchCursor {
			line = cursorRowStyle.Width(resultInnerWidth).MaxHeight(1).Render("▶ " + line)
		} else {
			line = "  " + line
		}
		resultLines = append(resultLines, line)
	}
	if len(resultLines) == 0 {
		resultLines = append(resultLines, lipgloss.NewStyle().Foreground(colorMuted).Render("  No results yet."))
	}
	resultsBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorCyan).
		Width(innerWidth).
		Height(maxResults + 2).
		MaxHeight(maxResults + 2).
		Render(strings.Join(resultLines, "\n"))

	help := lipgloss.NewStyle().Foreground(colorPhosphor).Render(
		truncateStr("󰐊 [ENTER] Search / Play + Radio | [TAB] Queue Only | 󰒭 [UP/DOWN] Select | 󰈆 [ESC] Close", innerWidth),
	)

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", inputView, statusView, resultsBox, help)
	modalBox := lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(colorPink).
		Padding(1, 1).
		Width(modalWidth).
		Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modalBox)
}

// FormatTime formats seconds into MM:SS matching app.py's format_time.
func FormatTime(seconds float64) string {
	if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return "00:00"
	}
	total := int(seconds)
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

func visibleWindow(cursor, total, height int) (int, int) {
	if total <= height {
		return 0, total
	}
	start := cursor - height/2
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > total {
		end = total
		start = end - height
	}
	return start, end
}

func truncateStr(s string, maxCols int) string {
	if maxCols <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxCols {
		return s
	}
	if maxCols <= 1 {
		return "…"
	}
	target := maxCols - 1
	var sb strings.Builder
	cols := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if cols+rw > target {
			break
		}
		sb.WriteRune(r)
		cols += rw
	}
	return sb.String() + "…"
}

func nonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
