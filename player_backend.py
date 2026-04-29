import mpv
import os

class MPVManager:
    def __init__(self, on_metadata_change=None, on_tick=None):
        # terminal=False and log_handler=None ensures mpv doesn't spam the TUI's stdout
        self.player = mpv.MPV(ytdl=True, video=False, terminal=False)
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

    def play(self, file_path):
        if os.path.exists(file_path):
            self.player.play(file_path)

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
            if 'icy-title' in metadata: # For streams/some files
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
