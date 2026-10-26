package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"tui-music/internal/library"
	"tui-music/internal/player"
	"tui-music/internal/ui"
	"tui-music/internal/youtube"
)

func printUsage() {
	fmt.Fprintf(os.Stdout, `tuimusic - Retro CRT terminal music player (Go port)

Usage:
  tuimusic [flags]
  tuimusic scan [--root <dir>] [--limit <n>] [--out <file>]

Commands:
  scan        Scan a music directory for audio files and write songs.json

Flags:
  --songs string   Path to songs.json library index (default "songs.json")
  -h, --help       Show this help message

Keybindings:
  SPACE            Toggle Play/Pause
  n                Next Track
  p                Previous Track
  s                Shuffle Queue
  /                Search YouTube Music
  TAB              Switch focus between Library Tree and Upcoming Queue
  ENTER            Expand/collapse folder or play selected track
  q                Quit
`)
}

func runScan(args []string) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	root := fs.String("root", "/Volumes/music", "Root directory to scan for audio files")
	limit := fs.Int("limit", 0, "Optional maximum number of songs to index (0 = no limit)")
	out := fs.String("out", "songs.json", "Output JSON file path")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// Support optional positional limit argument matching `python scanner.py [limit]`
	if *limit == 0 && fs.NArg() > 0 {
		if parsed, err := strconv.Atoi(fs.Arg(0)); err == nil && parsed > 0 {
			*limit = parsed
		}
	}

	scanRoot := *root
	if _, err := os.Stat(scanRoot); os.IsNotExist(err) {
		fmt.Printf("Error: %s not found. Please ensure your NAS is mounted.\n", scanRoot)
		if scanRoot == "/Volumes/music" {
			scanRoot = "./"
		} else {
			return 1
		}
	}

	songs, err := library.ScanLibrary(scanRoot, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning library: %v\n", err)
		return 1
	}

	if err := library.Save(*out, songs); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", *out, err)
		return 1
	}

	fmt.Printf("Successfully indexed %d songs into %s\n", len(songs), *out)
	return 0
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "scan" {
		os.Exit(runScan(os.Args[2:]))
	}

	var songsPath string
	var showHelp bool
	flag.StringVar(&songsPath, "songs", "songs.json", "Path to songs.json library index")
	flag.BoolVar(&showHelp, "help", false, "Show help")
	flag.BoolVar(&showHelp, "h", false, "Show help")
	flag.Usage = printUsage
	flag.Parse()

	if showHelp {
		printUsage()
		return
	}

	// Verify external binaries at startup with actionable installation instructions
	if _, err := player.CheckBinary(); err != nil {
		fmt.Fprintf(os.Stderr, "Startup check failed: %v\n", err)
		os.Exit(1)
	}
	if _, err := youtube.CheckBinary(); err != nil {
		fmt.Fprintf(os.Stderr, "Startup check failed: %v\n", err)
		os.Exit(1)
	}

	mpvClient, err := player.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start mpv backend: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		_ = mpvClient.Close()
	}()

	// Ensure clean shutdown on SIGINT/SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		_ = mpvClient.Close()
	}()

	var songs []library.Song
	if _, err := os.Stat(songsPath); err == nil {
		if loaded, loadErr := library.Load(songsPath); loadErr == nil {
			songs = loaded
		}
	}

	model := ui.NewModel(mpvClient, songs, youtube.Search)
	prog := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		_ = mpvClient.Close()
		fmt.Fprintf(os.Stderr, "Application error: %v\n", err)
		os.Exit(1)
	}
}
