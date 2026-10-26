package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"tui-music/internal/library"
	"tui-music/internal/player"
	"tui-music/internal/youtube"
)

type mockPlayer struct {
	playedSources []string
	paused        bool
	closed        bool
	updates       chan player.State
}

func newMockPlayer() *mockPlayer {
	return &mockPlayer{
		updates: make(chan player.State, 8),
	}
}

func (m *mockPlayer) Play(source string) error {
	m.playedSources = append(m.playedSources, source)
	m.paused = false
	return nil
}

func (m *mockPlayer) TogglePause() error {
	m.paused = !m.paused
	return nil
}

func (m *mockPlayer) Stop() error              { return nil }
func (m *mockPlayer) Seek(seconds float64) error { return nil }
func (m *mockPlayer) Duration() float64        { return 180.0 }
func (m *mockPlayer) TimePos() float64         { return 30.0 }
func (m *mockPlayer) Paused() bool             { return m.paused }
func (m *mockPlayer) Title() string            { return "Mock Title" }
func (m *mockPlayer) Artist() string           { return "Mock Artist" }
func (m *mockPlayer) PeakLevel() float64       { return 0.5 }
func (m *mockPlayer) Updates() <-chan player.State {
	return m.updates
}
func (m *mockPlayer) Close() error {
	m.closed = true
	return nil
}

func TestUIKeybindingsAndPlaybackControls(t *testing.T) {
	mp := newMockPlayer()
	songs := []library.Song{
		{Path: "/music/ArtistA/Album1/Song1.mp3", Title: "Song 1", Artist: "Artist A", Album: "Album 1"},
		{Path: "/music/ArtistB/Album2/Song2.mp3", Title: "Song 2", Artist: "Artist B", Album: "Album 2"},
		{Path: "/music/ArtistC/Album3/Song3.mp3", Title: "Song 3", Artist: "Artist C", Album: "Album 3"},
	}

	m := NewModel(mp, songs, func(ctx context.Context, query string) ([]youtube.Result, error) {
		return []youtube.Result{
			{VideoID: "yt123", Title: "YT Track", Artist: "YT Artist", Duration: "03:45"},
		}, nil
	})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	if !m.isPlaying {
		t.Fatalf("expected initial playback state to be playing")
	}

	// Space toggles pause
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if m.isPlaying || !mp.paused {
		t.Fatalf("expected space to pause playback")
	}

	// While paused, cassette reel index should not advance on tick
	initialReel := m.reelIndex
	for i := 0; i < 6; i++ {
		m.Update(tickMsg(time.Now()))
	}
	if m.reelIndex != initialReel {
		t.Fatalf("expected cassette reel not to spin while paused")
	}

	// Space resumes playback and ticks spin the cassette reels
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !m.isPlaying {
		t.Fatalf("expected space to resume playback")
	}
	for i := 0; i < 3; i++ {
		m.Update(tickMsg(time.Now()))
	}
	if m.reelIndex == initialReel {
		t.Fatalf("expected cassette reel to spin while playing")
	}

	// 'n' moves to next track, 'p' moves back to previous track
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.queue.CurrentIndex != 1 {
		t.Fatalf("expected CurrentIndex 1 after 'n', got %d", m.queue.CurrentIndex)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if m.queue.CurrentIndex != 0 {
		t.Fatalf("expected CurrentIndex 0 after 'p', got %d", m.queue.CurrentIndex)
	}

	// 's' shuffles queue while preserving current track
	curBefore, _ := m.queue.Current()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	curAfter, _ := m.queue.Current()
	if curBefore.Source != curAfter.Source {
		t.Fatalf("expected current track to remain %q after shuffle, got %q", curBefore.Source, curAfter.Source)
	}

	// '/' opens search modal
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.searchOpen {
		t.Fatalf("expected '/' to open search modal")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.searchOpen {
		t.Fatalf("expected ESC to close search modal")
	}

	// View renders Now Playing, cassette, and queue
	view := m.View()
	if !strings.Contains(view, "NASA TUI") || !strings.Contains(view, "UPCOMING QUEUE") {
		t.Fatalf("expected View() to render cassette and queue headers")
	}

	// Resize gate when terminal is too small
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	smallView := m.View()
	if !strings.Contains(smallView, "Terminal too small!") {
		t.Fatalf("expected resize gate on 40x10 window, got:\n%s", smallView)
	}

	// 'q' closes player and quits
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !mp.closed {
		t.Fatalf("expected 'q' to close player backend")
	}
}
