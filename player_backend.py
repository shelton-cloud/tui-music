import mpv
import os

class MPVManager:
    def __init__(self, on_metadata_change=None, on_tick=None):
        self.player = mpv.MPV(ytdl=True, video=False)
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
        if metadata and 'title' in metadata:
            return metadata['title']
        return os.path.basename(self.player.path) if self.player.path else "No Track"
