# TUI Music Player (`tuimusic`): Project Context

## Overview
`tui-music` (`tuimusic`) is a retro-styled, terminal-based music player built in **Go** using the [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss) frameworks. It integrates local NAS file playback (`nasa.local/music` across Linux and macOS) with YouTube Music streaming capabilities.

*(Note: The legacy Python/Textual prototype has been deprecated and moved to `deprecated/`.)*

## Architecture
- **Entrypoint (`cmd/tuimusic/main.go`):** CLI flag parsing (`--songs`, `--root`, `scan` subcommand), startup binary verification (`mpv` and `yt-dlp`), and signal-safe lifecycle management.
- **TUI Layer (`internal/ui/ui.go`):** Bubble Tea state machine rendering the side-by-side Library Tree (`Artist → Album → Track`) and Upcoming Queue, YouTube Music search modal, dynamic **animated cassette tape** widget, and Braille wave visualizer.
- **Playback Engine (`internal/player/player.go`):** Spawns headless `mpv` as a child process and communicates over a Unix domain socket using mpv's JSON IPC protocol.
- **Library & Scanner (`internal/library/library.go`):** Pure-Go audio metadata parser (`github.com/dhowden/tag`), `songs.json` persistence, and automatic cross-OS NAS mount detection (`DetectMusicRoot` / `ResolveSongs`) so macOS (`/Volumes/music`) and Linux (`nasa.local/music` via GVFS, `/mnt`, `/media`, or `$TUI_MUSIC_ROOT`) share the same `songs.json` seamlessly.
- **Streaming (`internal/youtube/youtube.go`):** Queries YouTube via `yt-dlp` and streams selected tracks through `mpv`.

## Prerequisites
1. **Go 1.22+** (for building)
2. **System mpv:**
   - Linux: `sudo apt install mpv`
   - macOS: `brew install mpv`
3. **yt-dlp:** Required for searching and streaming YouTube audio.

## Development Commands
- **Build Static Binary:**
  ```bash
  CGO_ENABLED=0 go build -o tuimusic ./cmd/tuimusic
  ```
- **Run Unit Tests:**
  ```bash
  go test ./...
  ```
- **Scan Local/NAS Library:**
  ```bash
  ./tuimusic scan [--root /path/to/music] [--limit N]
  ```
- **Run the Application:**
  ```bash
  ./tuimusic
  ```

## UI & Controls
- **SPACE:** Toggle Play/Pause (animation stops on pause).
- **N / P:** Next/Previous track in queue.
- **S:** Shuffle the current queue.
- **TAB:** Switch focus between Library Tree and Upcoming Queue panes.
- **ENTER:** Expand/collapse tree node or play selected track.
- **/ (Slash):** Open YouTube Music search modal.
- **Q:** Quit application.

