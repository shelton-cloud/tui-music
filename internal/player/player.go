package player

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// State represents a snapshot of the player's current playback state.
type State struct {
	TimePos   float64
	Duration  float64
	Paused    bool
	Title     string
	Artist    string
	Path      string
	PeakLevel float64
	EOF       bool
}

// CheckBinary verifies that the mpv executable is available in PATH.
func CheckBinary() (string, error) {
	p, err := exec.LookPath("mpv")
	if err != nil {
		installHint := "sudo apt install mpv"
		if runtime.GOOS == "darwin" {
			installHint = "brew install mpv"
		}
		return "", fmt.Errorf("required binary 'mpv' not found in PATH; install it with: %s", installHint)
	}
	return p, nil
}

// Client manages an mpv subprocess (or connects to an existing IPC socket) and
// speaks mpv's JSON IPC protocol over a Unix domain socket.
type Client struct {
	mu         sync.RWMutex
	conn       net.Conn
	cmd        *exec.Cmd
	socketPath string
	tempDir    string

	timePos   float64
	duration  float64
	paused    bool
	path      string
	metadata  map[string]string
	peakLevel float64

	updates chan State
	closed  chan struct{}
	closeMu sync.Once
}

// Start launches an mpv subprocess listening on a temporary Unix domain socket
// and connects the IPC client to it.
func Start() (*Client, error) {
	mpvBin, err := CheckBinary()
	if err != nil {
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "tuimusic-mpv-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir for mpv socket: %w", err)
	}
	sockPath := filepath.Join(tempDir, "mpv.sock")

	cmd := exec.Command(
		mpvBin,
		"--idle=yes",
		"--no-video",
		"--no-terminal",
		"--ytdl=yes",
		"--af=lavfi=[astats=metadata=1:reset=1]",
		"--input-ipc-server="+sockPath,
	)
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, fmt.Errorf("failed to start mpv: %w", err)
	}

	var conn net.Conn
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err = net.DialTimeout("unix", sockPath, 200*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	if conn == nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = os.RemoveAll(tempDir)
		return nil, fmt.Errorf("timed out waiting for mpv IPC socket at %s: %w", sockPath, err)
	}

	c := newClient(conn, sockPath, tempDir, cmd)
	if err := c.registerObservers(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

// Connect dials an existing mpv IPC Unix socket (used by unit tests with a fake server).
func Connect(socketPath string) (*Client, error) {
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to mpv socket %s: %w", socketPath, err)
	}
	c := newClient(conn, socketPath, "", nil)
	if err := c.registerObservers(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

func newClient(conn net.Conn, sockPath, tempDir string, cmd *exec.Cmd) *Client {
	c := &Client{
		conn:       conn,
		cmd:        cmd,
		socketPath: sockPath,
		tempDir:    tempDir,
		metadata:   make(map[string]string),
		updates:    make(chan State, 32),
		closed:     make(chan struct{}),
	}
	go c.readLoop()
	return c
}

func (c *Client) registerObservers() error {
	props := []string{
		"time-pos",
		"duration",
		"pause",
		"metadata",
		"path",
		"af-metadata",
	}
	for i, prop := range props {
		if err := c.sendCommand([]any{"observe_property", i + 1, prop}); err != nil {
			return fmt.Errorf("failed to observe %s: %w", prop, err)
		}
	}
	return nil
}

func (c *Client) sendCommand(args []any) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return errors.New("mpv IPC connection closed")
	}

	payload := map[string]any{
		"command": args,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Write(data)
	return err
}

// Play loads and starts playing a local file path or stream URL.
func (c *Client) Play(source string) error {
	c.mu.Lock()
	c.path = source
	c.paused = false
	c.timePos = 0
	c.mu.Unlock()

	if err := c.sendCommand([]any{"loadfile", source, "replace"}); err != nil {
		return err
	}
	return c.sendCommand([]any{"set_property", "pause", false})
}

// TogglePause toggles playback pause state.
func (c *Client) TogglePause() error {
	c.mu.Lock()
	c.paused = !c.paused
	paused := c.paused
	c.mu.Unlock()
	c.emitState(false)
	return c.sendCommand([]any{"set_property", "pause", paused})
}

// Stop stops playback.
func (c *Client) Stop() error {
	c.mu.Lock()
	c.timePos = 0
	c.path = ""
	c.mu.Unlock()
	return c.sendCommand([]any{"stop"})
}

// Seek seeks relative to the current playback position by seconds.
func (c *Client) Seek(seconds float64) error {
	return c.sendCommand([]any{"seek", seconds, "relative"})
}

// Duration returns the current track duration in seconds.
func (c *Client) Duration() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.duration
}

// TimePos returns the current playback position in seconds.
func (c *Client) TimePos() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.timePos
}

// Paused returns whether playback is currently paused.
func (c *Client) Paused() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.paused
}

// Title returns the current track title from metadata or falls back to the filename.
func (c *Client) Title() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.titleLocked()
}

func (c *Client) titleLocked() string {
	if t := c.lookupMetaLocked("title"); t != "" {
		return t
	}
	if t := c.lookupMetaLocked("icy-title"); t != "" {
		return t
	}
	if c.path != "" {
		return filepath.Base(c.path)
	}
	return "No Track Playing"
}

// Artist returns the current track artist from metadata or "Unknown Artist".
func (c *Client) Artist() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.artistLocked()
}

func (c *Client) artistLocked() string {
	if a := c.lookupMetaLocked("artist"); a != "" {
		return a
	}
	if a := c.lookupMetaLocked("album_artist"); a != "" {
		return a
	}
	return "Unknown Artist"
}

func (c *Client) lookupMetaLocked(key string) string {
	if v, ok := c.metadata[key]; ok && v != "" {
		return v
	}
	for k, v := range c.metadata {
		if strings.EqualFold(k, key) && v != "" {
			return v
		}
	}
	return ""
}

// PeakLevel returns the current cached audio peak level in [0.0, 1.0].
func (c *Client) PeakLevel() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.peakLevel
}

// Snapshot returns a copy of the current State.
func (c *Client) Snapshot() State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return State{
		TimePos:   c.timePos,
		Duration:  c.duration,
		Paused:    c.paused,
		Title:     c.titleLocked(),
		Artist:    c.artistLocked(),
		Path:      c.path,
		PeakLevel: c.peakLevel,
	}
}

// Updates returns a read-only channel of non-blocking player state updates.
func (c *Client) Updates() <-chan State {
	return c.updates
}

type ipcEvent struct {
	Event  string          `json:"event"`
	Name   string          `json:"name"`
	Data   json.RawMessage `json:"data"`
	Reason string          `json:"reason"`
}

func (c *Client) readLoop() {
	scanner := bufio.NewScanner(c.conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev ipcEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		switch ev.Event {
		case "property-change":
			c.handlePropertyChange(ev.Name, ev.Data)
		case "end-file":
			if ev.Reason == "eof" {
				c.emitState(true)
			}
		}
	}
}

func (c *Client) handlePropertyChange(name string, raw json.RawMessage) {
	if len(raw) == 0 || string(raw) == "null" {
		return
	}

	changed := false
	c.mu.Lock()
	switch name {
	case "time-pos":
		var v float64
		if json.Unmarshal(raw, &v) == nil {
			c.timePos = v
			changed = true
		}
	case "duration":
		var v float64
		if json.Unmarshal(raw, &v) == nil {
			c.duration = v
			changed = true
		}
	case "pause":
		var v bool
		if json.Unmarshal(raw, &v) == nil {
			c.paused = v
			changed = true
		}
	case "path":
		var v string
		if json.Unmarshal(raw, &v) == nil {
			c.path = v
			changed = true
		}
	case "metadata":
		var m map[string]any
		if json.Unmarshal(raw, &m) == nil {
			newMeta := make(map[string]string, len(m))
			for k, val := range m {
				newMeta[k] = fmt.Sprint(val)
			}
			c.metadata = newMeta
			changed = true
		}
	case "af-metadata":
		var m map[string]any
		if json.Unmarshal(raw, &m) == nil {
			if peak, ok := extractPeakFromAFMetadata(m); ok {
				c.peakLevel = peak
				changed = true
			}
		}
	}
	c.mu.Unlock()

	if changed {
		c.emitState(false)
	}
}

func extractPeakFromAFMetadata(m map[string]any) (float64, bool) {
	target := m
	if astats, ok := m["astats"].(map[string]any); ok {
		target = astats
	}
	peakKeys := []string{
		"lavfi.astats.Overall.Peak_level",
		"Overall.Peak_level",
		"lavfi.astats.1.Overall.Peak_level",
		"1.Overall.Peak_level",
	}
	for _, key := range peakKeys {
		if rawVal, exists := target[key]; exists {
			if level, ok := parsePeakValue(rawVal); ok {
				return level, true
			}
		}
	}
	return 0, false
}

func parsePeakValue(rawVal any) (float64, bool) {
	var val float64
	switch v := rawVal.(type) {
	case float64:
		val = v
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
			return 0, false
		}
		val = parsed
	default:
		return 0, false
	}

	var peak float64
	if val < 0 {
		peak = math.Pow(10, val/20.0)
	} else {
		peak = val
	}
	if peak < 0 {
		peak = 0
	}
	if peak > 1 {
		peak = 1
	}
	return peak, true
}

func (c *Client) emitState(eof bool) {
	snap := c.Snapshot()
	snap.EOF = eof
	select {
	case <-c.closed:
		return
	default:
	}
	select {
	case c.updates <- snap:
	default:
		// Drop oldest update so IPC reader never blocks
		select {
		case <-c.updates:
		default:
		}
		select {
		case c.updates <- snap:
		default:
		}
	}
}

// Close cleanly shuts down the mpv process, closes the IPC connection, and removes the temporary socket directory.
func (c *Client) Close() error {
	c.closeMu.Do(func() {
		close(c.closed)
		_ = c.sendCommand([]any{"quit"})

		c.mu.Lock()
		if c.conn != nil {
			_ = c.conn.Close()
			c.conn = nil
		}
		c.mu.Unlock()

		if c.cmd != nil && c.cmd.Process != nil {
			done := make(chan error, 1)
			go func() {
				done <- c.cmd.Wait()
			}()
			select {
			case <-done:
			case <-time.After(500 * time.Millisecond):
				_ = c.cmd.Process.Kill()
				<-done
			}
		}
		if c.tempDir != "" {
			_ = os.RemoveAll(c.tempDir)
		}
	})
	return nil
}
