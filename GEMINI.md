# TUI Music Player: Project Context

## Overview
`tui-music` is a retro-styled, terminal-based music player built with Python and the [Textual](https://textual.textualize.io/) framework. It integrates local file playback (optimized for NAS mounts) with YouTube Music streaming capabilities.

## Architecture
- **TUI Layer (`app.py`):** Handles the user interface, including the queue, search modals, and a dynamic **animated cassette tape** widget.
- **Playback Engine (`player_backend.py`):** A wrapper around `python-mpv`. It uses **automatic audio driver detection** for cross-platform compatibility and handles metadata/progress observation.
- **Data Ingestion (`scanner.py`):** Scans a root directory (e.g., a NAS mount) and extracts rich metadata (**Artist, Title, Album**) using the `mutagen` library, persisting results to `songs.json`.
- **Streaming:** Leverages `ytmusicapi` for searching and `yt-dlp` (via mpv's ytdl integration) for high-quality audio streaming.

## Tech Stack
- **Framework:** Textual (TUI)
- **Audio Engine:** MPV (via `python-mpv`)
- **Metadata/Search:** `ytmusicapi` & `mutagen` (local tags)
- **Streaming:** `yt-dlp`
- **Styling:** `rich` (integrated into Textual)

## Prerequisites
1. **Python 3.10+**
2. **System mpv:** 
   - macOS: `brew install mpv`
   - Linux: `sudo apt install mpv`
3. **yt-dlp:** Required for streaming playback.
4. **Mutagen:** Required for local metadata parsing (installed via `pip`).

## Development Commands
- **Install Dependencies:**
  ```bash
  python -m venv venv
  source venv/bin/activate
  pip install -r requirements.txt
  pip install mutagen  # Ensure mutagen is present
  ```
- **Scan Local Library:**
  ```bash
  python scanner.py [limit]
  ```
  *Note: You can pass an optional numeric limit to the scanner. Edit `scanner.py` to point to your NAS mount path.*
- **Run the Application:**
  ```bash
  python app.py
  ```

## UI & Controls
- **SPACE:** Toggle Play/Pause. (Animation stops on pause).
- **N / P:** Next/Previous track in queue.
- **S:** Shuffle the current queue.
- ** / (Slash):** Open YouTube Music search modal.
- **Q:** Quit application.

## Conventions
- **Queue Shuffling:** The application automatically shuffles the entire library and loads a random selection of 200 tracks on startup.
- **Metadata Priority:** Local files use internal tags (ID3/FLAC/etc.) first, falling back to the filename if tags are missing.
- **Visuals:** Double-line box drawing and animated reels (Braille/Block characters) define the "Cassette" aesthetic.
