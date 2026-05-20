import os
import mpv

class MPVManager:
    def __init__(self, on_metadata_change=None, on_tick=None):
        print("DEBUG: Initializing MPV...")
        try:
            # Let MPV autodetect the audio output (ao) for better compatibility
            self.player = mpv.MPV(ytdl=True, video=False, terminal=False)
            print(f"DEBUG: MPV Initialized. Driver: {self.player.ao}")
        except Exception as e:
            print(f"DEBUG: MPV Initialization failed: {e}")
            raise

        self.on_metadata_change = on_metadata_change
        self.on_tick = on_tick

        # Observe properties
        @self.player.property_observer('metadata')
        def metadata_observer(_name, value):
            if self.on_metadata_change:
                self.on_metadata_change(value)

        @self.player.property_observer('time-pos')
        def time_observer(_name, value):
            if self.on_tick:
                self.on_tick(value)

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
