# TUI Music Player 🎵

A retro-styled terminal music player that connects to your NAS and streams from YouTube Music.

## Features
- **NAS Integration**: Browse your local music library via a directory tree.
- **YouTube Music Streaming**: Search and queue songs directly from YouTube Music.
- **Retro UI**: CRT-inspired colors and a cassette tape animation.
- **MPV Powered**: High-quality playback with support for various audio formats and streaming.

## Prerequisites
- **Python 3.10+**
- **mpv**: Required for audio playback. Install via `brew install mpv` (macOS) or `sudo apt install mpv` (Linux).
- **yt-dlp**: Required for streaming.

## Installation
1. Clone the repository.
2. Create a virtual environment: `python -m venv venv`.
3. Install dependencies: `./venv/bin/pip install -r requirements.txt`.
4. Run the app: `./venv/bin/python app.py`.

## Controls
- `SPACE`: Play/Pause
- `N`: Next Track
- `P`: Previous Track
- `/`: Search YouTube Music
- `Q`: Quit

## Configuration
- Update `scanner.py` with your NAS mount path and run it to index local files into `songs.json`.
- The player defaults to `./` if the NAS is not found.
