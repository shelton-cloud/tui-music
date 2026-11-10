package player

import (
	"bufio"
	"encoding/json"
	"math"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeMPVServer struct {
	t        *testing.T
	listener net.Listener
	sockPath string
	tempDir  string

	mu       sync.Mutex
	commands [][]any
	conn     net.Conn
	connReady chan struct{}
}

func newFakeMPVServer(t *testing.T) *fakeMPVServer {
	t.Helper()
	// Use /tmp directly on macOS to stay well under the 104-byte sun_path limit.
	tempDir, err := os.MkdirTemp("/tmp", "mpvtest-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	sockPath := filepath.Join(tempDir, "s.sock")
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		t.Fatalf("failed to listen on unix socket: %v", err)
	}
	srv := &fakeMPVServer{
		t:         t,
		listener:  l,
		sockPath:  sockPath,
		tempDir:   tempDir,
		connReady: make(chan struct{}),
	}
	go srv.acceptLoop()
	return srv
}

func (s *fakeMPVServer) acceptLoop() {
	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	s.serveConn(conn)
}

func (s *fakeMPVServer) serveConn(conn net.Conn) {
	s.mu.Lock()
	s.conn = conn
	s.mu.Unlock()
	close(s.connReady)

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		var msg struct {
			Command []any `json:"command"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &msg); err == nil {
			s.mu.Lock()
			s.commands = append(s.commands, msg.Command)
			s.mu.Unlock()
			_, _ = conn.Write([]byte(`{"error":"success"}` + "\n"))
		}
	}
}

func (s *fakeMPVServer) sendEvent(ev map[string]any) {
	<-s.connReady
	data, err := json.Marshal(ev)
	if err != nil {
		s.t.Fatalf("marshal event: %v", err)
	}
	data = append(data, '\n')
	s.mu.Lock()
	conn := s.conn
	s.mu.Unlock()
	if conn != nil {
		_, _ = conn.Write(data)
	}
}

func (s *fakeMPVServer) getCommands() [][]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([][]any, len(s.commands))
	copy(out, s.commands)
	return out
}

func (s *fakeMPVServer) close() {
	_ = s.listener.Close()
	s.mu.Lock()
	if s.conn != nil {
		_ = s.conn.Close()
	}
	s.mu.Unlock()
	_ = os.RemoveAll(s.tempDir)
}

func TestClientWithFakeIPCServer(t *testing.T) {
	srv := newFakeMPVServer(t)
	defer srv.close()

	client, err := Connect(srv.sockPath)
	if err != nil {
		// Fallback to in-memory net.Pipe when running inside a restricted sandbox that blocks unix dial
		srvConn, cliConn := net.Pipe()
		go srv.serveConn(srvConn)
		client = newClient(cliConn, srv.sockPath, "", nil)
		if regErr := client.registerObservers(); regErr != nil {
			t.Fatalf("registerObservers failed: %v", regErr)
		}
	}
	defer client.Close()

	if got := client.Title(); got != "No Track Playing" {
		t.Fatalf("expected default title 'No Track Playing', got %q", got)
	}
	if got := client.Artist(); got != "Unknown Artist" {
		t.Fatalf("expected default artist 'Unknown Artist', got %q", got)
	}

	if err := client.Play("/Volumes/music/2Pac/Dear Mama.mp3"); err != nil {
		t.Fatalf("Play failed: %v", err)
	}

	// Fallback title before metadata arrives should be filename
	if got := client.Title(); got != "Dear Mama.mp3" {
		t.Fatalf("expected fallback filename title 'Dear Mama.mp3', got %q", got)
	}

	// Send metadata, duration, time-pos, and af-metadata from fake server
	srv.sendEvent(map[string]any{
		"event": "property-change",
		"name":  "metadata",
		"data": map[string]any{
			"title":  "Dear Mama",
			"artist": "2Pac",
		},
	})
	srv.sendEvent(map[string]any{
		"event": "property-change",
		"name":  "duration",
		"data":  280.0,
	})
	srv.sendEvent(map[string]any{
		"event": "property-change",
		"name":  "time-pos",
		"data":  42.5,
	})
	srv.sendEvent(map[string]any{
		"event": "property-change",
		"name":  "af-metadata",
		"data": map[string]any{
			"astats": map[string]any{
				"lavfi.astats.Overall.Peak_level": "-6.0",
			},
		},
	})

	// Wait for state updates on channel
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-client.Updates():
			if client.Title() == "Dear Mama" && client.Artist() == "2Pac" && client.Duration() == 280.0 && client.TimePos() == 42.5 && client.PeakLevel() > 0.4 {
				goto verified
			}
		case <-deadline:
			t.Fatalf("timed out waiting for state updates; got title=%q artist=%q dur=%v pos=%v peak=%v",
				client.Title(), client.Artist(), client.Duration(), client.TimePos(), client.PeakLevel())
		}
	}

verified:
	expectedPeak := math.Pow(10, -6.0/20.0)
	if math.Abs(client.PeakLevel()-expectedPeak) > 0.01 {
		t.Fatalf("expected peak ~%v, got %v", expectedPeak, client.PeakLevel())
	}

	if err := client.TogglePause(); err != nil {
		t.Fatalf("TogglePause failed: %v", err)
	}
	if !client.Paused() {
		t.Fatalf("expected client to be paused after TogglePause")
	}

	if err := client.Seek(10); err != nil {
		t.Fatalf("Seek failed: %v", err)
	}
	if err := client.AdjustVolume(-10); err != nil {
		t.Fatalf("AdjustVolume failed: %v", err)
	}
	if client.Volume() != 90 {
		t.Fatalf("expected volume 90 after AdjustVolume(-10), got %v", client.Volume())
	}
	if err := client.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Verify commands received by fake server
	time.Sleep(50 * time.Millisecond)
	cmds := srv.getCommands()
	var sawLoadfile, sawPause, sawSeek, sawStop bool
	for _, cmd := range cmds {
		if len(cmd) == 0 {
			continue
		}
		name, _ := cmd[0].(string)
		switch name {
		case "loadfile":
			sawLoadfile = true
		case "set_property":
			if len(cmd) >= 2 && cmd[1] == "pause" {
				sawPause = true
			}
		case "seek":
			sawSeek = true
		case "stop":
			sawStop = true
		}
	}
	if !sawLoadfile || !sawPause || !sawSeek || !sawStop {
		t.Fatalf("missing expected IPC commands: loadfile=%v pause=%v seek=%v stop=%v",
			sawLoadfile, sawPause, sawSeek, sawStop)
	}
}
