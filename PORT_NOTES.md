# Architecture Notes (`tuimusic`)

## Overview

`tuimusic` is a single static Go executable (`CGO_ENABLED=0`) built with Bubble Tea and Lip Gloss. The original Python/Textual prototype has been retired in favor of this pure-Go architecture.

### Why mpv JSON IPC Instead of `libmpv` (cgo)
- **Zero-cgo Static Binary (`CGO_ENABLED=0`)**: Binding `libmpv` via cgo ties compilation and runtime linking to platform-specific C headers and dynamic library paths.
- **Process Isolation & Format Parity**: Launching the system `mpv` binary as a child process (`mpv --idle=yes --no-video --no-terminal --ytdl=yes --cache=yes --af=lavfi=[astats=metadata=1:reset=1] --input-ipc-server=<unix-socket>`) and communicating via JSON IPC over a temporary Unix domain socket provides full audio codec and `yt-dlp` streaming support with zero C bindings.
- **Non-Blocking Event Stream**: `internal/player.Client` subscribes to `time-pos`, `duration`, `pause`, `metadata`, `path`, `af-metadata`, and `volume` via `observe_property` and pushes state snapshots onto a buffered Go channel consumed by Bubble Tea `tea.Cmd` subscriptions so the UI loop never blocks on socket I/O.

### Package Layout
- [`cmd/tuimusic/main.go`](cmd/tuimusic/main.go): Entrypoint, CLI flag parsing (`--songs`, `--root`), `scan` subcommand, startup binary verification (`mpv` and `yt-dlp`), NAS availability detection, and signal-safe cleanup.
- [`internal/player/`](internal/player/player.go): `mpv` subprocess lifecycle, Unix socket JSON IPC client, volume/seek controls, property observers, and fake-IPC-server unit tests.
- [`internal/library/`](internal/library/library.go): `songs.json` loader/writer, cross-OS NAS path resolver (`DetectMusicRoot`, `ResolveSongs`), persistent hybrid YT/NAS Favorites Mixtape (`favorites.json`), pure-Go audio tag scanner (`github.com/dhowden/tag`), collapsible `Artist → Album → Track` tree model, and playback queue state machine.
- [`internal/youtube/`](internal/youtube/youtube.go): Direct YouTube Music `WEB_REMIX` Innertube search & `RDAMVM` Auto-Radio queue generation (with `yt-dlp` fallback), plus `lrclib.net` time-synced LRC lyrics fetching and parsing.
- [`internal/ui/`](internal/ui/ui.go): Bubble Tea (`github.com/charmbracelet/bubbletea`) + Lip Gloss (`github.com/charmbracelet/lipgloss`) retro CRT interface, dual-mode left pane (YT Mixtape vs. NAS Tree), animated cassette deck, synced lyrics display, Braille wave visualiser, and YouTube Music search modal.
