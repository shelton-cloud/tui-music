# TUI Music Player (`tuimusic`): Project Context

## Overview
`tui-music` (`tuimusic`) is a retro-styled, terminal-based music player built in **Go** using the [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss) frameworks. It uses **YouTube Music** as the primary streaming and discovery source (with Auto-Radio, Mixtape favorites, and synced lyrics) alongside **local NAS file playback** (`nasa.local/music` across Linux and macOS) as a secondary source.

## Architecture
- **Entrypoint (`cmd/tuimusic/main.go`):** CLI flag parsing (`--songs`, `--root`, `scan` subcommand), startup binary verification (`mpv` and `yt-dlp`), NAS reachability check, and signal-safe lifecycle management.
- **TUI Layer (`internal/ui/ui.go`):** Bubble Tea state machine rendering the dual-mode left pane (`[1] YT Music & Mixtape` vs. `[2] NAS Artist → Album → Track Tree`), the Upcoming Queue, YouTube Music search modal, dynamic **animated cassette tape** widget, synced lyrics display, and Braille wave visualizer.
- **Playback Engine (`internal/player/player.go`):** Spawns headless `mpv` (`--ytdl=yes --cache=yes`) as a child process and communicates over a Unix domain socket using mpv's JSON IPC protocol (observing `time-pos`, `duration`, `pause`, `metadata`, `path`, `af-metadata`, and `volume`).
- **YouTube Music & Lyrics (`internal/youtube/youtube.go`):** Queries YouTube Music's `WEB_REMIX` Innertube `search` and `next` (`RDAMVM` Auto-Radio) endpoints directly over HTTP with automatic `yt-dlp` fallback, and fetches time-synced LRC lyrics from `lrclib.net`.
- **Library, Favorites & Scanner (`internal/library/library.go`):** Pure-Go audio metadata parser (`github.com/dhowden/tag`), `songs.json` persistence, hybrid YT/NAS favorites persistence (`~/.config/tuimusic/favorites.json`), and automatic cross-OS NAS path translation.

## Tech Stack
- **Language:** Go (`CGO_ENABLED=0` static binary)
- **TUI Framework:** Bubble Tea, Bubbles, Lip Gloss (`charmbracelet`)
- **Audio Engine:** System `mpv` via Unix socket JSON IPC
- **Metadata/Tags:** `github.com/dhowden/tag` (pure Go ID3/FLAC/M4A/OGG)
- **Streaming & Discovery:** YouTube Music Innertube HTTP API + `yt-dlp` (via `mpv`) + `lrclib.net` (synced lyrics)

## Prerequisites
1. **Go 1.22+**
2. **System mpv:**
   - macOS: `brew install mpv`
   - Linux: `sudo apt install mpv`
3. **yt-dlp:** Required for YouTube Music audio streaming (`brew install yt-dlp` or `sudo apt install yt-dlp`).

## Development Commands
- **Build Static Binary:**
  ```bash
  CGO_ENABLED=0 go build -o tuimusic ./cmd/tuimusic
  ```
- **Run Unit Tests & Vet:**
  ```bash
  go vet ./...
  go test -v ./...
  ```
- **Scan Local NAS Library:**
  ```bash
  ./tuimusic scan [--root /path/to/music] [--limit N]
  ```
- **Run the Application:**
  ```bash
  ./tuimusic
  ```

## UI & Controls
- **`/` (Slash):** Open YouTube Music search modal (`ENTER` plays immediately + starts Auto-Radio; `TAB` queues without interrupting).
- **`SPACE`:** Toggle Play/Pause (reel animation stops on pause).
- **`N` / `P`:** Next / Previous track in queue.
- **`S`:** Shuffle the current queue.
- **`R`:** Toggle YouTube Music Auto-Radio (`[RADIO:ON/OFF]`).
- **`F`:** Toggle track in persistent Favorites Mixtape (`★`).
- **`L`:** Toggle real-time Synced Lyrics in the Now Playing panel.
- **`D` / `X`:** Remove highlighted track from the queue.
- **`-` / `+`:** Adjust volume ($\pm 5\%$).
- **`,` / `.`:** Seek backward / forward ($\pm 5\text{s}$).
- **`1` / `2` / `M`:** Switch Left Pane between YT Mixtape (`1`) and NAS Library Tree (`2`).
- **`TAB`:** Switch focus between Left Pane and Upcoming Queue.
- **`Q`:** Quit application.

## Conventions
- **YouTube Music Primary / NAS Secondary:** When the NAS mount is unavailable at startup, `tuimusic` starts cleanly in YouTube Music & Mixtape mode without queueing unplayable local paths.
- **Metadata Priority:** YouTube streams preserve clean YT Music `Title`, `Artist`, and `Album` metadata over raw `mpv` stream filenames. Local files use embedded tags first, falling back to directory/filename structure.
- **Visuals:** Double-line box drawing, CRT phosphor accents, and animated reels (`◐`, `◓`, `◑`, `◒`) define the "Cassette" aesthetic.
