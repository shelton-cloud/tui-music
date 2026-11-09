import os
import sys
import ctypes.util

# Robust fix for macOS Homebrew libmpv location
if sys.platform == 'darwin':
    HOMEBREW_LIB = "/opt/homebrew/lib/libmpv.dylib"
    if os.path.exists(HOMEBREW_LIB):
        # Monkey-patch find_library so 'import mpv' can find it
        _original_find_library = ctypes.util.find_library
        def _new_find_library(name):
            if name == 'mpv':
                return HOMEBREW_LIB
            return _original_find_library(name)
        ctypes.util.find_library = _new_find_library

# pylint: disable=wrong-import-position
import mpv

class MPVManager:
    def __init__(self, on_metadata_change=None, on_tick=None):
        print("DEBUG: Initializing MPV...")
        try:
            # Let MPV autodetect the audio output (ao) for better compatibility
            self.player = mpv.MPV(ytdl=True, video=False, terminal=False)
            # Add audio filter to track stats (volume peaks)
            self.player.af = 'lavfi=[astats=metadata=1:reset=1]'
            print(f"DEBUG: MPV Initialized. Driver: {self.player.ao}")
        except Exception as e:
            print(f"DEBUG: MPV Initialization failed: {e}")
            raise

        self.on_metadata_change = on_metadata_change
        self.on_tick = on_tick
        self._peak_level = 0.0

        @self.player.property_observer('time-pos')
        def time_observer(_name, value):
            if self.on_tick:
                self.on_tick(value)

        # Observe audio filter metadata (real-time)
        @self.player.property_observer('af-metadata')
        def metadata_observer(_name, value):
            if not isinstance(value, dict):
                return

            # Look for astats data
            target_data = value.get('astats', value)
            if not isinstance(target_data, dict):
                return

            peak_keys = ['lavfi.astats.Overall.Peak_level', 'lavfi.astats.1.Overall.Peak_level']
            for key in peak_keys:
                # Check both full key and stripped key
                for k in [key, key.replace('lavfi.astats.', '')]:
                    if k in target_data:
                        self._update_peak(target_data[k])
                        return

        # Also keep track of tag changes
        @self.player.property_observer('metadata')
        def tags_observer(_name, value):
            if self.on_metadata_change:
                self.on_metadata_change(value)

    def _update_peak(self, raw_val):
        """Update internal peak level from raw value (dB or ratio)."""
        try:
            val = float(raw_val)
            if val < 0: # dB
                self._peak_level = 10**(val/20)
            else: # Ratio
                self._peak_level = val
        except (ValueError, TypeError):
            pass

    def play(self, source):
        # source can be a local file path or a URL
        self.player.play(source)

    def toggle_pause(self):
        self.player.pause = not self.player.pause

    def stop(self):
        self.player.stop()

    def seek(self, seconds):
        self.player.seek(seconds)

    @property
    def duration(self):
        return self.player.duration

    @property
    def title(self):
        # Extract title from metadata or filename
        metadata = self.player.metadata
        if metadata:
            if 'title' in metadata:
                return metadata['title']
            if 'icy-title' in metadata:  # For streams/some files
                return metadata['icy-title']
        return os.path.basename(self.player.path) if self.player.path else "No Track Playing"

    @property
    def artist(self):
        metadata = self.player.metadata
        if metadata:
            if 'artist' in metadata:
                return metadata['artist']
            if 'album_artist' in metadata:
                return metadata['album_artist']
        return "Unknown Artist"

    @property
    def peak_level(self):
        """Returns the current cached peak level (0.0 to 1.0)."""
        return self._peak_level
