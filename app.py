from textual.app import App, ComposeResult
from textual.containers import Horizontal, Vertical, Container
from textual.widgets import Header, Footer, DirectoryTree, ListView, ListItem, Label, ProgressBar, Static
from textual.binding import Binding
from textual import work
import os
import json
import random
from player_backend import MPVManager

class TrackItem(ListItem):
    def __init__(self, file_path):
        super().__init__()
        self.file_path = file_path
        self.title = os.path.basename(file_path)

    def compose(self) -> ComposeResult:
        yield Label(self.title)

class NowPlaying(Static):
    CASSETTE = r"""
  ________________
 / [O]        [O] \
|    TUI MUSIC     |
|   [========]     |
 \________________/
"""

    def __init__(self):
        super().__init__()
        self.track_title = "No Track Playing"

    def compose(self) -> ComposeResult:
        with Horizontal():
            yield Static(self.CASSETTE, id="cassette-art")
            with Vertical(id="track-info-container"):
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
        background: #0b0914;
    }
    #main-container {
        height: 1fr;
    }
    #breadcrumb-bar {
        background: #1a1b26;
        color: #7dcfff;
        padding: 0 1;
        height: 1;
        text-style: italic;
    }
    DirectoryTree {
        width: 35%;
        border: solid #b14dff;
        background: #1a1b26;
        color: #00e5ff;
    }
    DirectoryTree > .directory-tree--extension {
        color: #f92aad;
    }
    DirectoryTree:focus {
        border: double #00e5ff;
        background: #1e1c31;
    }
    #right-pane {
        width: 65%;
    }
    #queue-container {
        height: 60%;
        border: solid #b14dff;
        margin-left: 1;
        background: #131122;
    }
    #queue-header {
        text-style: bold;
        color: #f92aad;
        padding: 0 1;
        background: #1a1b26;
    }
    NowPlaying {
        height: 40%;
        margin-top: 1;
        margin-left: 1;
        border: solid #00e5ff;
        padding: 1;
        background: #16142a;
    }
    #status-bar {
        height: 1;
        background: #f92aad;
        color: #0b0914;
        text-style: bold;
    }
    #cassette-art {
        color: #f92aad;
        width: 20;
    }
    #track-info-container {
        width: 1fr;
        padding-left: 2;
    }
    #track-title {
        text-style: bold;
        color: #00e5ff;
        margin-bottom: 1;
    }
    ProgressBar {
        width: 100%;
    }
    ProgressBar > .progress-bar--bar {
        color: #f92aad;
        background: #333;
    }
    #track-time {
        color: #b14dff;
        text-align: right;
    }
    #resize-gate {
        display: none;
        height: 100%;
        content-align: center middle;
        text-style: bold;
        color: #f7768e;
        background: #0b0914;
    }
    """

    BINDINGS = [
        Binding("q", "quit", "Quit", show=True),
        Binding("space", "toggle_pause", "Play/Pause", show=True),
        Binding("n", "next_track", "Next", show=True),
        Binding("p", "prev_track", "Prev", show=True),
        Binding("slash", "search", "Search", show=True),
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
        yield Header(show_clock=True)
        yield Label("NAS > MUSIC", id="breadcrumb-bar")
        with Horizontal(id="main-container"):
            initial_path = "/Volumes/music" if os.path.exists("/Volumes/music") else "./"
            yield DirectoryTree(initial_path)
            with Vertical(id="right-pane"):
                with Container(id="queue-container"):
                    yield Label(" 🎵 UPCOMING QUEUE ", id="queue-header")
                    yield ListView(id="queue-list")
                yield NowPlaying()
        yield Static("Terminal too small! Please resize your pane.\n(Minimum 60x15 required)", id="resize-gate")
        yield Label(" [SPACE] Pause | [N] Next | [P] Prev | [/] Search | [Q] Quit ", id="status-bar")
        yield Footer()

    def on_resize(self, event) -> None:
        # Show/Hide resize gate based on terminal size
        too_small = event.size.width < 60 or event.size.height < 15
        self.query_one("#main-container").display = not too_small
        self.query_one("#resize-gate").display = too_small
        self.query_one("#breadcrumb-bar").display = not too_small
        self.query_one("#status-bar").display = not too_small

    def on_mount(self) -> None:
        if os.path.exists("songs.json"):
            try:
                with open("songs.json", "r") as f:
                    all_songs = json.load(f)
                if all_songs:
                    count = min(len(all_songs), 50)
                    random_selection = random.sample(all_songs, count)
                    for song in random_selection:
                        self.add_to_queue(song, play_now=False)
                    self.play_track(0)
            except Exception:
                pass

    def on_directory_tree_file_selected(self, event: DirectoryTree.FileSelected) -> None:
        file_path = str(event.path)
        if file_path.lower().endswith(('.mp3', '.flac', '.wav', '.m4a', '.ogg')):
            self.add_to_queue(file_path)
        # Update breadcrumb
        self.query_one("#breadcrumb-bar", Label).update(f"NAS > {os.path.dirname(file_path).split('/')[-1].upper()} > {os.path.basename(file_path)}")

    def add_to_queue(self, file_path, play_now=True):
        self.queue.append(file_path)
        self.query_one("#queue-list", ListView).append(TrackItem(file_path))
        if play_now and self.current_index == -1:
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
