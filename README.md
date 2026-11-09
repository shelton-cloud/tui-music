# TUI Music Player (`tuimusic`) 🎵

A retro-styled CRT terminal music player built in Go (Bubble Tea + Lip Gloss) that connects to your NAS (`nasa.local/music`) and streams from YouTube Music via `mpv` and `yt-dlp`.

> **Note:** The original Python implementation (`app.py`, `scanner.py`, `player_backend.py`) has been deprecated and archived under `deprecated/` in favor of the single static Go binary (`tuimusic`).

## Features
- **Zero-CGO Static Binary**: Compiles to a single self-contained executable (`tuimusic`).
- **Cross-OS NAS Integration**: Browse your local music library (`Artist → Album → Track`) with automatic path resolution across Linux (`smb://nasa.local/music` via GVFS, `/mnt`, `/media`) and macOS (`/Volumes/music`).
- **YouTube Music Streaming**: Search and queue songs directly from YouTube Music via `yt-dlp`.
- **Retro CRT UI**: Dracula/Phosphor CRT colors, an animated cassette deck (`NASA TUI`), and a Braille wave visualizer.
- **MPV JSON IPC**: High-quality playback powered by an isolated `mpv` background process over a Unix socket.

## Prerequisites
- **Go 1.22+** (to build from source)
- **mpv**: Required for audio playback (`sudo apt install mpv` on Linux or `brew install mpv` on macOS).
- **yt-dlp**: Required for YouTube Music search and streaming (`sudo apt install yt-dlp` or `brew install yt-dlp`).

## Building & Running

```bash
# Build the static Go binary
CGO_ENABLED=0 go build -o tuimusic ./cmd/tuimusic

# Run the player (auto-detects your Linux/macOS NAS mount path)
./tuimusic

# Or explicitly pass your NAS mount root if mounted in a custom location:
./tuimusic --root /path/to/nasa.local/music
```

## Indexing Your NAS Library (`scan`)

`tuimusic` automatically translates `/Volumes/music/...` paths in `songs.json` to your active Linux NAS mount at runtime, so you can share the same `songs.json` between macOS and Linux without re-scanning.

When you add new music to your NAS and want to re-index:

```bash
# Auto-detects nasa.local/music (GVFS, /mnt/music, /Volumes/music, or $TUI_MUSIC_ROOT)
./tuimusic scan

# Or specify a custom mount directory and optional song limit:
./tuimusic scan --root /run/user/1000/gvfs/smb-share:server=nasa.local,share=music --limit 500
```

## Controls
- `SPACE`: Play/Pause
- `N`: Next Track
- `P`: Previous Track
- `S`: Shuffle Queue
- `/`: Search YouTube Music
- `TAB`: Switch focus between Library Tree and Upcoming Queue
- `ENTER`: Expand/collapse folder or play selected track
- `Q`: Quit

See [PORT_NOTES.md](PORT_NOTES.md) for architecture details.

