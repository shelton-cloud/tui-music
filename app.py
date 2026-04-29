from textual.app import App, ComposeResult
from textual.containers import Horizontal, Vertical, Container
from textual.widgets import Header, Footer, DirectoryTree, ListView, ListItem, Label, ProgressBar, Static
from textual.binding import Binding
from textual import work
import os
from player_backend import MPVManager

class TrackItem(ListItem):
    def __init__(self, file_path):
        super().__init__()
        self.file_path = file_path
        self.title = os.path.basename(file_path)

    def compose(self) -> ComposeResult:
        yield Label(self.title)

class NowPlaying(Static):
    def __init__(self):
        super().__init__()
        self.track_title = "No Track Playing"
        self.progress = 0.0

    def compose(self) -> ComposeResult:
        with Vertical(id="now-playing-container"):
            yield Label(self.track_title, id="track-title")
            yield ProgressBar(total=100, show_percentage=True, id="track-progress")
            yield Label("00:00 / 00:00", id="track-time")

    def update_info(self, title, progress, time_str):
        self.query_one("#track-title", Label).update(title)
        self.query_one("#track-progress", ProgressBar).progress = progress
        self.query_one("#track-time", Label).update(time_str)

class MusicPlayerApp(App):
    CSS = """
    Screen {
        background: #1a1b26;
    }
    #main-container {
        height: 100%;
    }
    DirectoryTree {
        width: 40%;
        border-right: solid #333;
        background: #1a1b26;
    }
    #right-pane {
        width: 60%;
    }
    #queue-container {
        height: 70%;
        border-bottom: solid #333;
    }
    #now-playing-container {
        height: 30%;
        padding: 1;
        align: center middle;
    }
    #track-title {
        text-style: bold;
        color: #7aa2f7;
        margin-bottom: 1;
    }
    ProgressBar {
        width: 100%;
        color: #bb9af7;
    }
    """

    BINDINGS = [
        Binding("q", "quit", "Quit", show=True),
        Binding("space", "toggle_pause", "Play/Pause", show=True),
        Binding("n", "next_track", "Next", show=True),
        Binding("p", "prev_track", "Prev", show=True),
    ]

    def __init__(self):
        super().__init__()
        self.queue = []
        self.current_index = -1
        self.player = MPVManager(
            on_metadata_change=self.handle_metadata,
            on_tick=self.handle_tick
        )

    def compose(self) -> ComposeResult:
        yield Header()
        with Horizontal(id="main-container"):
            # Start at NAS mount if it exists, otherwise current dir
            initial_path = "/Volumes/music" if os.path.exists("/Volumes/music") else "./"
            yield DirectoryTree(initial_path)
            with Vertical(id="right-pane"):
                with Container(id="queue-container"):
                    yield Label("Queue", id="queue-header")
                    yield ListView(id="queue-list")
                yield NowPlaying()
        yield Footer()

    def on_directory_tree_file_selected(self, event: DirectoryTree.FileSelected) -> None:
        file_path = str(event.path)
        if file_path.lower().endswith(('.mp3', '.flac', '.wav', '.m4a', '.ogg')):
            self.add_to_queue(file_path)

    def add_to_queue(self, file_path):
        self.queue.append(file_path)
        self.query_one("#queue-list", ListView).append(TrackItem(file_path))
        if self.current_index == -1:
            self.play_track(0)

    def play_track(self, index):
        if 0 <= index < len(self.queue):
            self.current_index = index
            self.player.play(self.queue[index])
            # Highlight in list
            queue_list = self.query_one("#queue-list", ListView)
            queue_list.index = index

    def action_toggle_pause(self):
        self.player.toggle_pause()

    def action_next_track(self):
        if self.current_index + 1 < len(self.queue):
            self.play_track(self.current_index + 1)

    def action_prev_track(self):
        if self.current_index - 1 >= 0:
            self.play_track(self.current_index - 1)

    def handle_metadata(self, metadata):
        self.call_from_thread(self.update_now_playing_title)

    def handle_tick(self, time_pos):
        self.call_from_thread(self.update_progress, time_pos)

    def update_now_playing_title(self):
        title = self.player.title
        now_playing = self.query_one(NowPlaying)
        now_playing.track_title = title # Simple update for now
        self.query_one("#track-title", Label).update(title)

    def update_progress(self, time_pos):
        if time_pos is None: return
        duration = self.player.duration
        if duration:
            percent = (time_pos / duration) * 100
            time_str = f"{self.format_time(time_pos)} / {self.format_time(duration)}"
            now_playing = self.query_one(NowPlaying)
            now_playing.update_info(self.player.title, percent, time_str)
            
            # Auto-next
            if time_pos >= duration - 0.5:
                self.action_next_track()

    def format_time(self, seconds):
        if seconds is None: return "00:00"
        m, s = divmod(int(seconds), 60)
        return f"{m:02d}:{s:02d}"

if __name__ == "__main__":
    app = MusicPlayerApp()
    app.run()
