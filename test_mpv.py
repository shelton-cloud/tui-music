import mpv
import sys

try:
    print("Attempting to initialize MPV...")
    player = mpv.MPV(ytdl=True, video=False, terminal=False)
    print("MPV initialized successfully!")
    sys.exit(0)
except Exception as e:
    print(f"FAILED to initialize MPV: {e}")
    sys.exit(1)
