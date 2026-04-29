# TUI Music Player

A lightweight, colorful TUI music player built with Python, Textual, and MPV.

## Features
- **File Browser:** Navigate your local library or NAS mounts.
- **Queue System:** Add tracks to a live queue by selecting them in the browser.
- **Now Playing:** Real-time progress bar and track metadata.
- **Async Backend:** Powered by `python-mpv` for high-quality, non-blocking playback.

## Setup
1. **Install MPV:** (System dependency)
   ```bash
   brew install mpv
   ```
2. **Setup Environment:**
   ```bash
   python3 -m venv venv
   source venv/bin/activate
   pip install -r requirements.txt
   ```
3. **Run:**
   ```bash
   python app.py
   ```

## Keybindings
- `space`: Play / Pause
- `n`: Next track in queue
- `p`: Previous track in queue
- `enter`: (In Browser) Add file to queue
- `q`: Quit app
