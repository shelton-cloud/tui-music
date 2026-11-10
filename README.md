# TUI Music Player (`tuimusic`) 🎵

A retro-styled CRT terminal music player built in **Go** ([Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss)) with **YouTube Music** as the primary streaming & discovery engine and **local NAS playback** (`nasa.local/music`) as a seamless secondary source.

## Features
- **Zero-CGO Static Binary**: Compiles to a single self-contained Go executable (`tuimusic`) with no C bindings required.
- **YouTube Music Primary + Auto-Radio**:
  - Fast YouTube Music search via the pure-Go Innertube API (with `yt-dlp` fallback).
  - **Auto-Radio (`R`)**: Picking any YouTube Music track automatically queues up related radio tracks (`RDAMVM<videoId>`) in the background so the music never stops.
- **Hybrid Mixtape Favorites (`F`)**: Dub any YouTube Music stream or local NAS track to your persistent Mixtape (`~/.config/tuimusic/favorites.json`).
- **Time-Synced Lyrics (`L`)**: Fetches synced LRC lyrics from `lrclib.net` and highlights the current lyric line in real time in the Now Playing deck.
- **Cross-OS NAS Integration (`1` / `2` / `M`)**: Browse your local NAS library (`Artist → Album → Track`) with automatic path resolution across Linux (`smb://nasa.local/music` via GVFS, `/mnt`, `/media`) and macOS (`/Volumes/music`). Automatically falls back to YouTube Music & Mixtape mode when away from your NAS.
- **Retro CRT UI**: Dracula/Phosphor CRT palette, an animated `NASA TUI` cassette deck, and a high-resolution Braille wave visualizer driven by `mpv` JSON IPC.

## Prerequisites
- **Go 1.22+** (to build from source)
- **mpv**: Required for audio playback (`sudo apt install mpv` on Linux or `brew install mpv` on macOS).
- **yt-dlp**: Required for YouTube Music audio stream extraction (`sudo apt install yt-dlp` or `brew install yt-dlp`).

## Building & Running

```bash
# Build the static Go binary
CGO_ENABLED=0 go build -o tuimusic ./cmd/tuimusic

# Run the player (starts in YT Music + Mixtape mode if NAS is offline, or loads NAS library when mounted)
./tuimusic

# Or explicitly pass a custom NAS mount root or songs.json path:
./tuimusic --root /path/to/nasa.local/music --songs songs.json
```

## Indexing Your NAS Library (`scan`)

`tuimusic` automatically translates `/Volumes/music/...` paths in `songs.json` to your active Linux or macOS NAS mount at runtime, so you can share the same `songs.json` across machines without re-scanning.

When you add new music to your NAS and want to re-index:

```bash
# Auto-detects nasa.local/music (GVFS, /mnt/music, /Volumes/music, or $TUI_MUSIC_ROOT)
./tuimusic scan

# Or specify a custom mount directory and optional song limit:
./tuimusic scan --root /Volumes/music --limit 500
```

## Controls
| Key | Action |
| :--- | :--- |
| `/` | Search YouTube Music (`ENTER` to play immediately + start Auto-Radio, `TAB` to queue next) |
| `SPACE` | Toggle Play / Pause (pauses cassette reel animation) |
| `N` / `P` | Next / Previous track in queue |
| `S` | Shuffle current queue |
| `R` | Toggle YouTube Music Auto-Radio (`[RADIO:ON/OFF]`) |
| `F` | Save / remove current or highlighted track in Favorites Mixtape (`★`) |
| `L` | Toggle real-time Synced Lyrics in the Now Playing deck |
| `D` / `X` | Remove highlighted track from the Upcoming Queue |
| `-` / `+` | Volume Down / Up ($\pm 5\%$) |
| `,` / `.` | Seek Backward / Forward ($\pm 5\text{s}$) |
| `1` / `2` / `M` | Switch Left Pane between `[1] YT Music & Mixtape` and `[2] NAS Library Tree` |
| `TAB` | Switch focus between Left Pane and Upcoming Queue |
| `ENTER` | Play selected track or expand/collapse folder in NAS Tree |
| `Q` | Quit application |

See [PORT_NOTES.md](PORT_NOTES.md) for architecture details.
