# Go Port Notes (`tuimusic`)

## Architecture Overview

This port brings `tui-music` from Python (`textual` + `python-mpv` + `mutagen` + `ytmusicapi`) to a single static Go executable (`CGO_ENABLED=0`) while preserving full feature parity, keybindings, and `songs.json` compatibility.

### Why mpv JSON IPC Instead of `libmpv` (cgo)
- **Zero-cgo Static Binary (`CGO_ENABLED=0`)**: Binding `libmpv` via cgo ties compilation and runtime linking to platform-specific C headers and dynamic library paths (which previously required `ctypes.util.find_library` monkey-patching on macOS for `/opt/homebrew/lib/libmpv.dylib`).
- **Process Isolation & Format Parity**: Launching the system `mpv` binary as a child process (`mpv --idle=yes --no-video --no-terminal --ytdl=yes --af=lavfi=[astats=metadata=1:reset=1] --input-ipc-server=<unix-socket>`) and communicating via JSON IPC over a temporary Unix domain socket provides identical audio codec and `yt-dlp` streaming support with zero C bindings.
- **Non-Blocking Event Stream**: `internal/player.Client` subscribes to `time-pos`, `duration`, `pause`, `metadata`, `path`, and `af-metadata` via `observe_property` and pushes state snapshots onto a buffered Go channel consumed by Bubble Tea `tea.Cmd` subscriptions so the UI loop never blocks on socket I/O.

### Package Layout
- [`cmd/tuimusic/main.go`](cmd/tuimusic/main.go): Entrypoint, CLI flag parsing, `scan` subcommand, startup binary verification (`mpv` and `yt-dlp`), and signal-safe cleanup.
- [`internal/player/`](internal/player/player.go): `mpv` subprocess lifecycle, Unix socket JSON IPC client, property observers, and fake-IPC-server unit tests.
- [`internal/library/`](internal/library/library.go): `songs.json` loader/writer (4-space indented JSON matching Python's `json.dump`), pure-Go audio tag scanner (`github.com/dhowden/tag`), collapsible `Artist → Album → Track` tree model, and playback queue state machine.
- [`internal/youtube/`](internal/youtube/youtube.go): `yt-dlp --dump-single-json --flat-playlist "ytsearch5:<query>"` search wrapper and JSON parser.
- [`internal/ui/`](internal/ui/ui.go): Bubble Tea (`github.com/charmbracelet/bubbletea`) + Lip Gloss (`github.com/charmbracelet/lipgloss`) retro CRT interface, animated cassette deck, Braille wave visualiser, and YouTube Music search modal.

---

## Parity Checklist

- [x] **Browse the local library as a directory tree derived from `songs.json` paths (artist → album → track)**: Implemented in `internal/library.Tree` and rendered in the left library pane (`TAB` switches focus between the Library Tree and Upcoming Queue panes; `ENTER` expands/collapses Artist/Album nodes or plays a Track).
- [x] **Select a track and play it via mpv IPC**: Selecting a track in either the Library Tree or Upcoming Queue sends `loadfile` and unpauses via mpv JSON IPC.
- [x] **`space` toggles pause/resume; `n`/`p` move through the queue; `s` shuffles**: All original keybindings are preserved (`q`, `space`, `n`, `p`, `s`, `/`), and shuffling keeps the currently playing track index aligned.
- [x] **Now-playing pane: title, artist, album, elapsed/total time, progress**: Displays track title, artist, album, playback status, progress bar with percentage, and `MM:SS / MM:SS` elapsed/total time.
- [x] **`/` opens a search box that queries YouTube Music through `yt-dlp` and queues the selection**: Opens the modal search overlay, queries `yt-dlp`, displays `[duration] title - artist` results, and queues/plays the chosen stream on `ENTER`.
- [x] **Queue display with current-track highlight**: Renders the Upcoming Queue with `▶` and phosphor-green highlight on the active track.
- [x] **Retro CRT styling: dark background, phosphor-green/amber accents, and a cassette-tape animation driven by playback state**: Uses the CRT dark palette (`#282a36`) with phosphor-green (`#50fa7b`), amber (`#ffb86c`), and magenta (`#ff79c6`) accents, plus the double-line `NASA TUI` cassette with reels (`◐`, `◓`, `◑`, `◒`) spinning while playing and stopped when paused.
- [x] **Visualiser**: Implements the exact 2x4 Braille dot-matrix (`U+2800` offset) wave renderer from `app.py`. It observes `af-metadata` (`lavfi.astats.Overall.Peak_level`) over mpv IPC; **Limitation note**: some `mpv` builds/configurations do not emit `af-metadata` updates over the JSON IPC socket at high frequency (or emit empty `af-metadata` on certain audio drivers). When `af-metadata` peak levels are zero/unavailable during active playback, the visualiser automatically falls back to a determinate multi-harmonic wave animation while playing and a low-amplitude idle sine wave when paused, rather than faking an FFT spectrum.
- [x] **Clean shutdown: mpv process killed, IPC socket removed, terminal restored**: `mpvClient.Close()` sends `quit`, closes the socket, waits/kills the child `mpv` process, removes the temporary socket directory, and Bubble Tea exits alt-screen cleanly on `q` or `SIGINT`/`SIGTERM`.

---

## Building and Running

### Build Static Binary
```bash
CGO_ENABLED=0 go build -o tuimusic ./cmd/tuimusic
```

### Run the TUI Player
```bash
./tuimusic
```
By default, `tuimusic` reads `songs.json` from the current directory (configurable via `--songs /path/to/songs.json`).

### Scan Local Music Library
```bash
./tuimusic scan --root /Volumes/music
# Or with an optional song limit:
./tuimusic scan --root /Volumes/music --limit 500
```

---

## Intentional Differences from the Python Version

1. **YouTube Search via `yt-dlp` Instead of `ytmusicapi`**: Since `ytmusicapi` is Python-only, searches invoke `yt-dlp --dump-single-json --flat-playlist --no-warnings "ytsearch5:<query>"` and stream the selected video URL via `mpv`.
2. **Pure-Go Tag Parsing (`github.com/dhowden/tag`)**: Replaces `mutagen` for ID3v1/v2, FLAC, MP4/M4A, and Ogg Vorbis metadata extraction while retaining identical fallback defaults (`filepath.Base(path)`, `"Unknown Artist"`, `"Unknown Album"`) and 4-space JSON formatting in `songs.json`.
3. **Side-by-Side Library Tree & Queue Panes**: Exposes the `Artist → Album → Track` tree derived from `songs.json` alongside the Upcoming Queue (`TAB` switches pane focus, `UP`/`DOWN`/`ENTER` navigate and select) without shadowing any of the core keybindings (`q`, `space`, `n`, `p`, `s`, `/`).
