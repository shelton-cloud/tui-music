package ui

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

// SearchFunc resolves a YouTube Music search query into results.
type SearchFunc func(ctx context.Context, query string) ([]youtube.Result, error)

type paneFocus int

const (
	focusQueue paneFocus = iota
	focusTree
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
	colorBg         = lipgloss.Color("#282a36")
	colorDarkBg     = lipgloss.Color("#1e1f29")
	colorHeaderBg   = lipgloss.Color("#343746")
	colorPhosphor   = lipgloss.Color("#50fa7b")
	colorAmber      = lipgloss.Color("#ffb86c")
	colorPink       = lipgloss.Color("#ff79c6")
	colorCyan       = lipgloss.Color("#8be9fd")
	colorPurple     = lipgloss.Color("#bd93f9")
	colorMuted      = lipgloss.Color("#6272a4")
	colorAlertRed   = lipgloss.Color("#ff5555")

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

// Model is the Bubble Tea state model for tui-music.
type Model struct {
	player     PlayerBackend
	searchFn   SearchFunc
	allSongs   []library.Song
	tree       *library.Tree
	queue      *library.Queue
	queueIndex int
	focus      paneFocus

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
	if searchFn == nil {
		searchFn = youtube.Search
	}
	ti := textinput.New()
	ti.Placeholder = "Search for a song..."
	ti.CharLimit = 120
	ti.Width = 50

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	q := library.NewQueue()
	q.LoadInitialFromLibrary(songs, rng)

	m := &Model{
		player:      p,
		searchFn:    searchFn,
		allSongs:    songs,
		tree:        library.NewTree(songs),
		queue:       q,
		focus:       focusQueue,
		width:       80,
		height:      24,
		breadcrumb:  "󰎆 NASA TUI",
		trackTitle:  "No Track Playing",
		trackArtist: "",
		trackAlbum:  "",
		waveHistory: make([]float64, 100),
		searchInput: ti,
		rng:         rng,
	}

	if len(m.queue.Items) > 0 {
		m.playTrack(0)
	}
	return m
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
		m.applyPlayerState(player.State(msg))
		return m, waitForPlayerState(m.player)

	case searchResultsMsg:
		if msg.err != nil {
			m.searchStatus = fmt.Sprintf("Search error: %v", msg.err)
		} else {
			m.searchResults = msg.results
			m.searchCursor = 0
			m.searchStatus = fmt.Sprintf("Found %d results", len(msg.results))
		}
		return m, nil

	case tea.KeyMsg:
		if m.searchOpen {
			return m.updateSearchModal(msg)
		}
		return m.updateMainView(msg)
	}

	return m, nil
}

func (m *Model) updateSearchModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

	case "enter":
		// If results are already displayed and the user navigated or submitted the same query,
		// allow Enter on a highlighted result when input isn't modified, or run search if results are empty.
		query := strings.TrimSpace(m.searchInput.Value())
		if len(m.searchResults) > 0 && m.searchCursor >= 0 && m.searchCursor < len(m.searchResults) && m.searchStatus != "Searching..." {
			selected := m.searchResults[m.searchCursor]
			m.searchOpen = false
			m.searchInput.Blur()
			m.addTrackToQueue(selected.ToTrack(), true)
			return m, nil
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

func (m *Model) updateMainView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		if m.player != nil {
			_ = m.player.Close()
		}
		return m, tea.Quit

	case " ":
		m.actionTogglePause()
		return m, nil

	case "n":
		m.actionNextTrack()
		return m, nil

	case "p":
		m.actionPrevTrack()
		return m, nil

	case "s":
		m.actionShuffle()
		return m, nil

	case "/":
		m.searchOpen = true
		m.searchStatus = "Press ENTER to search YouTube Music, then ENTER again to queue"
		m.searchResults = nil
		m.searchCursor = 0
		m.searchInput.SetValue("")
		m.searchInput.Focus()
		return m, nil

	case "tab", "left", "right":
		if m.focus == focusQueue {
			m.focus = focusTree
		} else {
			m.focus = focusQueue
		}
		return m, nil

	case "up":
		if m.focus == focusTree {
			m.tree.MoveUp()
		} else if m.queueIndex > 0 {
			m.queueIndex--
		}
		return m, nil

	case "down":
		if m.focus == focusTree {
			m.tree.MoveDown()
		} else if m.queueIndex+1 < len(m.queue.Items) {
			m.queueIndex++
		}
		return m, nil

	case "enter":
		if m.focus == focusTree {
			if song := m.tree.Activate(); song != nil {
				track := library.TrackFromSong(*song)
				m.addTrackToQueue(track, true)
				dirName := strings.ToUpper(filepath.Base(filepath.Dir(song.Path)))
				m.breadcrumb = fmt.Sprintf("NAS > %s > %s", dirName, filepath.Base(song.Path))
			}
		} else {
			m.playTrack(m.queueIndex)
		}
		return m, nil
	}

	return m, nil
}

func (m *Model) addTrackToQueue(track library.Track, playNow bool) {
	wasEmpty := m.queue.CurrentIndex == -1
	started := m.queue.Add(track, playNow)
	if started || (playNow && wasEmpty) {
		m.playTrack(len(m.queue.Items) - 1)
	}
}

func (m *Model) playTrack(index int) {
	m.playTrackWithSkips(index, 0)
}

func (m *Model) playTrackWithSkips(index int, skips int) {
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
		if _, err := os.Stat(track.Source); err != nil {
			// Check if any file in the queue exists before skipping everything on an unmounted NAS
			if m.anyLocalFileExists() && index+1 < len(m.queue.Items) {
				m.isTransitioning = false
				m.playTrackWithSkips(index+1, skips+1)
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
			continue
		}
		if _, err := os.Stat(item.Source); err == nil {
			return true
		}
		checked++
		if checked >= 10 {
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

func (m *Model) actionNextTrack() {
	if m.isTransitioning {
		return
	}
	if m.queue.CurrentIndex+1 < len(m.queue.Items) {
		m.isTransitioning = true
		m.playTrack(m.queue.CurrentIndex + 1)
	}
}

func (m *Model) actionPrevTrack() {
	if m.queue.CurrentIndex-1 >= 0 {
		m.playTrack(m.queue.CurrentIndex - 1)
	}
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

func (m *Model) applyPlayerState(st player.State) {
	m.timePos = st.TimePos
	if st.Duration > 0 {
		m.duration = st.Duration
	}
	m.isPlaying = !st.Paused
	m.peakLevel = st.PeakLevel

	if st.Title != "" && st.Title != "No Track Playing" {
		m.trackTitle = st.Title
	}
	if st.Artist != "" && st.Artist != "Unknown Artist" {
		m.trackArtist = st.Artist
	}
	if cur, ok := m.queue.Current(); ok && m.trackAlbum == "" {
		m.trackAlbum = cur.Album
	}

	// Auto-next on EOF or within 0.8s of duration (matching app.py line 590)
	if !m.isTransitioning {
		if st.EOF || (m.duration > 0 && m.timePos >= m.duration-0.8) {
			m.actionNextTrack()
		}
	}
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
func (m *Model) View() string {
	if m.width < 60 || m.height < 15 {
		gateStyle := lipgloss.NewStyle().
			Width(m.width).
			Height(m.height).
			Align(lipgloss.Center, lipgloss.Center).
			Bold(true).
			Foreground(colorAlertRed).
			Background(colorBg)
		return gateStyle.Render("Terminal too small! Please resize your pane.\n(Min 60x15 required)")
	}

	if m.searchOpen {
		return m.renderSearchModal()
	}

	breadcrumb := breadcrumbStyle.Width(m.width).Render(m.breadcrumb)

	// Calculate vertical split matching app.py (60% top library/queue panes, 40% NowPlaying)
	availHeight := m.height - 2 // breadcrumb + status bar
	if availHeight < 12 {
		availHeight = 12
	}
	nowPlayingHeight := 10
	topHeight := availHeight - nowPlayingHeight
	if topHeight < 5 {
		topHeight = 5
	}

	topPane := m.renderTopPanes(m.width, topHeight)
	nowPlaying := m.renderNowPlaying(m.width, nowPlayingHeight)

	statusText := " 󰍉 [/] Search | 󰐊 [SPACE] Pause | 󰒭 [N] Next | 󰒮 [P] Prev | 󰒝 [S] Shuffle | [TAB] Tree/Queue | 󰈆 [Q] Quit "
	statusBar := statusBarStyle.Width(m.width).Render(truncateStr(statusText, m.width-2))

	return lipgloss.JoinVertical(lipgloss.Left, breadcrumb, topPane, nowPlaying, statusBar)
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

	treeBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(treeBorderColor).
		Width(leftWidth - 2).
		Height(innerHeight)

	queueBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(queueBorderColor).
		Width(rightWidth - 2).
		Height(innerHeight)

	treeContent := m.renderTreeList(leftWidth-4, innerHeight-1, treeHdrStyle)
	queueContent := m.renderQueueList(rightWidth-4, innerHeight-1, queueHdrStyle)

	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		treeBoxStyle.Render(treeContent),
		queueBoxStyle.Render(queueContent),
	)
}

func (m *Model) renderTreeList(width, listHeight int, hdrStyle lipgloss.Style) string {
	header := hdrStyle.Width(width).Render(" 󰙅 LIBRARY (Artist → Album → Track) ")
	if listHeight < 1 {
		return header
	}
	if len(m.tree.Visible) == 0 {
		return header + "\n" + lipgloss.NewStyle().Foreground(colorMuted).Render(" (No songs loaded in songs.json)")
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
			line = cursorRowStyle.Width(width).Render(line)
		}
		rows = append(rows, line)
	}
	return strings.Join(rows, "\n")
}

func (m *Model) renderQueueList(width, listHeight int, hdrStyle lipgloss.Style) string {
	header := hdrStyle.Width(width).Render(" 󰎆 UPCOMING QUEUE ")
	if listHeight < 1 {
		return header
	}
	if len(m.queue.Items) == 0 {
		return header + "\n" + lipgloss.NewStyle().Foreground(colorMuted).Render(" (Queue is empty)")
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
		line := truncateStr(fmt.Sprintf("%s%s - %s", prefix, item.Title, item.Artist), width)
		switch {
		case i == m.queueIndex && m.focus == focusQueue:
			line = cursorRowStyle.Width(width).Render(line)
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
	waveBlock := waveStyle.Render(RenderBrailleWave(m.waveHistory, 22, 2))
	leftColumn := lipgloss.JoinVertical(lipgloss.Left, cassetteBlock, waveBlock)

	infoWidth := totalWidth - 30
	if infoWidth < 20 {
		infoWidth = 20
	}

	stateBadge := "[PAUSED]"
	if m.isPlaying {
		stateBadge = "[PLAYING]"
	}
	titleLine := titleStyle.Render(truncateStr(m.trackTitle, infoWidth))
	artistLine := artistStyle.Render(truncateStr("Artist: "+nonEmpty(m.trackArtist, "Unknown Artist"), infoWidth))
	albumLine := albumStyle.Render(truncateStr("Album:  "+nonEmpty(m.trackAlbum, "Unknown Album")+"  "+stateBadge, infoWidth))

	var pct float64
	if m.duration > 0 {
		pct = math.Max(0, math.Min(100, (m.timePos/m.duration)*100))
	}
	bar := renderProgressBar(pct, infoWidth-8)
	timeStr := fmt.Sprintf("%s / %s", FormatTime(m.timePos), FormatTime(m.duration))
	timeLine := artistStyle.Width(infoWidth).Align(lipgloss.Right).Render(timeStr)

	rightColumn := lipgloss.NewStyle().PaddingLeft(2).Render(
		lipgloss.JoinVertical(lipgloss.Left, titleLine, artistLine, albumLine, "", bar, timeLine),
	)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(colorPhosphor).
		Width(totalWidth - 2)

	return boxStyle.Render(lipgloss.JoinHorizontal(lipgloss.Top, leftColumn, rightColumn))
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

	title := lipgloss.NewStyle().Bold(true).Foreground(colorPink).Render("󰍉 SEARCH YOUTUBE MUSIC")
	inputView := m.searchInput.View()
	statusView := lipgloss.NewStyle().Foreground(colorAmber).Render(m.searchStatus)

	var resultLines []string
	maxResults := modalHeight - 8
	if maxResults < 3 {
		maxResults = 3
	}
	start, end := visibleWindow(m.searchCursor, len(m.searchResults), maxResults)
	for i := start; i < end; i++ {
		r := m.searchResults[i]
		line := truncateStr(fmt.Sprintf("[%s] %s - %s", r.Duration, r.Title, r.Artist), modalWidth-6)
		if i == m.searchCursor {
			line = cursorRowStyle.Width(modalWidth - 6).Render("▶ " + line)
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
		Width(modalWidth - 4).
		Height(maxResults).
		Render(strings.Join(resultLines, "\n"))

	help := lipgloss.NewStyle().Foreground(colorPhosphor).Render(" 󰐊 [ENTER] Search/Queue | 󰒭 [UP/DOWN] Select | 󰈆 [ESC] Close ")

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

func truncateStr(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes <= 1 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-1]) + "…"
}

func nonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
