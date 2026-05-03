from textual.app import App, ComposeResult
from textual.containers import Horizontal, Vertical, Container
from textual.widgets import Header, Footer, DirectoryTree, ListView, ListItem, Label, ProgressBar, Static, Input
from textual.screen import ModalScreen
from textual.binding import Binding
from textual import work, on
import os
import json
import random
from player_backend import MPVManager
from ytmusicapi import YTMusic

class TrackItem(ListItem):
    def __init__(self, track_data):
        super().__init__()
        self.track_data = track_data
        # track_data can be a dict (Local File from scanner.py) or a dict (YT metadata)
        if isinstance(track_data, dict) and "path" in track_data:
            # Local file from scanner.py
            self.title = track_data.get('title', 'Unknown')
            self.source = track_data['path']
            self.artist = track_data.get('artist', 'Unknown Artist')
            self.album = track_data.get('album', 'Unknown Album')
        elif isinstance(track_data, str):
            # Fallback for raw strings
            self.title = os.path.basename(track_data)
            self.source = track_data
            self.artist = "Local File"
            self.album = "Unknown Album"
        else:
            # YouTube Music data
            self.title = track_data.get('title', 'Unknown')
            self.source = f"https://www.youtube.com/watch?v={track_data['videoId']}"
            self.artist = track_data.get('artists', [{'name': 'Unknown'}])[0]['name']
            self.album = "YouTube Music"

    def compose(self) -> ComposeResult:
        yield Label(f"{self.title} - {self.artist}")

class ResultItem(ListItem):
    def __init__(self, result):
        super().__init__()
        self.result = result
        self.title = result.get('title', 'Unknown')
        self.artist = result.get('artists', [{'name': 'Unknown'}])[0]['name']
        self.duration = result.get('duration', '??:??')

    def compose(self) -> ComposeResult:
        yield Label(f"[{self.duration}] {self.title} - {self.artist}")

class SearchScreen(ModalScreen):
    CSS = """
    SearchScreen {
        align: center middle;
    }
    #search-container {
        width: 80%;
        height: 80%;
        background: #1a1b26;
        border: thick #f92aad;
        padding: 1;
    }
    #search-results {
        height: 1fr;
        border: solid #00e5ff;
        margin-top: 1;
    }
    """

    def __init__(self):
        super().__init__()
        self.yt = YTMusic()

    def compose(self) -> ComposeResult:
        with Vertical(id="search-container"):
            yield Label("SEARCH YOUTUBE MUSIC", id="search-title")
            yield Input(placeholder="Search for a song...", id="search-input")
            yield Label("", id="search-status")
            yield ListView(id="search-results")
            yield Label(" [ENTER] Search | [UP/DOWN] Select | [ESC] Close ", id="search-help", markup=False)

    @on(Input.Submitted)
    def perform_search(self, event: Input.Submitted):
        query = event.value
        if query:
            self.query_one("#search-status", Label).update("Searching...")
            self.run_search(query)

    @work(exclusive=True, thread=True)
    def run_search(self, query):
        results = self.yt.search(query, filter="songs")
        self.app.call_from_thread(self.update_results, results)

    def update_results(self, results):
        self.query_one("#search-status", Label).update(f"Found {len(results)} results")
        search_list = self.query_one("#search-results", ListView)
        search_list.clear()
        for res in results:
            search_list.append(ResultItem(res))

    @on(ListView.Selected)
    def on_result_selected(self, event: ListView.Selected):
        if isinstance(event.item, ResultItem):
            self.dismiss(event.item.result)

class NowPlaying(Static):
    CASSETTE_FRAME = [
        " ╔══════════════════╗ ",
        " ║  TUI MUSIC PLAYER ║ ",
        " ╟──┬────────────┬──╢ ",
        " ║  │ {left}      {right} │  ║ ",
        " ║  ╰────────────╯  ║ ",
        " ╚══════╗    ╔══════╝ ",
        "        ╚════╝        "
    ]
    
    # Reel animation frames (Braille/Block transitions)
    REEL_FRAMES = ["◐", "◓", "◑", "◒"]
    
    def __init__(self):
        super().__init__()
        self.track_title = "No Track Playing"
        self.track_artist = ""
        self.reel_index = 0
        self.is_playing = False

    def on_mount(self) -> None:
        self.set_interval(0.2, self.animate_reels)

    def animate_reels(self) -> None:
        if self.is_playing:
            self.reel_index = (self.reel_index + 1) % len(self.REEL_FRAMES)
            self.update_cassette()

    def update_cassette(self) -> None:
        reel = self.REEL_FRAMES[self.reel_index]
        # Mirror the reels for symmetry
        left_reel = f"[bold #f92aad]{reel}[/]"
        right_reel = f"[bold #f92aad]{reel}[/]"
        
        cassette_art = "\n".join(self.CASSETTE_FRAME).format(
            left=left_reel, 
            right=right_reel
        )
        self.query_one("#cassette-art", Static).update(cassette_art)

    def compose(self) -> ComposeResult:
        with Horizontal():
            yield Static("", id="cassette-art")
            with Vertical(id="track-info-container"):
                yield Label(self.track_title, id="track-title")
                yield Label(self.track_artist, id="track-artist")
                yield ProgressBar(total=100, show_percentage=True, id="track-progress")
                yield Label("00:00 / 00:00", id="track-time")

    def update_info(self, title, artist, progress, time_str, is_playing=True):
        self.track_title = title
        self.track_artist = artist
        self.is_playing = is_playing
        self.query_one("#track-title", Label).update(title)
        self.query_one("#track-artist", Label).update(artist)
        self.query_one("#track-progress", ProgressBar).progress = progress
        self.query_one("#track-time", Label).update(time_str)
        if not self.is_playing:
            self.update_cassette()

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
    #queue-container {
        height: 60%;
        border: solid #b14dff;
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
        border: solid #00e5ff;
        padding: 1;
        background: #16142a;
        layout: horizontal;
    }
    #status-bar {
        height: 1;
        background: #f92aad;
        color: #0b0914;
        text-style: bold;
    }
    #cassette-art {
        color: #f92aad;
        width: 25;
    }
    #track-info-container {
        width: 1fr;
        padding-left: 2;
    }
    #track-title {
        text-style: bold;
        color: #00e5ff;
    }
    #track-artist {
        color: #b14dff;
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
        Binding("s", "shuffle", "Shuffle", show=True),
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
        yield Label("TUI MUSIC STREAMER", id="breadcrumb-bar")
        with Vertical(id="main-container"):
            with Container(id="queue-container"):
                yield Label(" 🎵 UPCOMING QUEUE ", id="queue-header")
                yield ListView(id="queue-list")
            yield NowPlaying()
        yield Static("Terminal too small! Please resize your pane.\n(Minimum 60x15 required)", id="resize-gate")
        yield Label(" [/] Search | [SPACE] Pause | [N] Next | [P] Prev | [Q] Quit ", id="status-bar", markup=False)
        yield Footer()

    def on_resize(self, event) -> None:
        # Show/Hide resize gate based on terminal size
        too_small = event.size.width < 60 or event.size.height < 15
        self.query_one("#main-container").display = not too_small
        self.query_one("#resize-gate").display = too_small
        self.query_one("#breadcrumb-bar").display = not too_small
        self.query_one("#status-bar").display = not too_small

    def on_mount(self) -> None:
        self.query_one(NowPlaying).update_cassette()
        if os.path.exists("songs.json"):
            try:
                with open("songs.json", "r") as f:
                    all_songs = json.load(f)
                if all_songs:
                    # Shuffle the entire library thoroughly
                    random.shuffle(all_songs)
                    
                    queue_list = self.query_one("#queue-list", ListView)
                    # Load the whole library into the queue
                    for song in all_songs:
                        track_item = TrackItem(song)
                        self.queue.append(track_item)
                        queue_list.append(track_item)
                    
                    if self.queue:
                        self.play_track(0)
            except Exception as e:
                # Log or handle error if needed, but for TUI we'll stay quiet
                pass

    def action_shuffle(self):
        if not self.queue:
            return
        
        # Save reference to current track data
        current_data = self.queue[self.current_index].track_data if self.current_index != -1 else None
        
        # Get all track data from current queue
        queue_data = [item.track_data for item in self.queue]
        random.shuffle(queue_data)
        
        # Clear UI and internal list
        self.query_one("#queue-list", ListView).clear()
        self.queue = []
        
        # Rebuild queue with shuffled data
        for data in queue_data:
            self.add_to_queue(data, play_now=False)
            
        # Try to find where the previously playing track moved to
        if current_data:
            for i, item in enumerate(self.queue):
                # Compare paths for local files or videoIds for YT
                is_match = False
                if isinstance(current_data, dict) and isinstance(item.track_data, dict):
                    if "path" in current_data and "path" in item.track_data:
                        is_match = current_data["path"] == item.track_data["path"]
                    elif "videoId" in current_data and "videoId" in item.track_data:
                        is_match = current_data["videoId"] == item.track_data["videoId"]
                
                if is_match:
                    self.current_index = i
                    self.query_one("#queue-list", ListView).index = i
                    break
        else:
            self.play_track(0)

    def action_search(self):
        self.push_screen(SearchScreen(), self.handle_search_result)

    def handle_search_result(self, result):
        if result:
            self.add_to_queue(result)

    def on_directory_tree_file_selected(self, event: DirectoryTree.FileSelected) -> None:
        file_path = str(event.path)
        if file_path.lower().endswith(('.mp3', '.flac', '.wav', '.m4a', '.ogg')):
            self.add_to_queue(file_path)
        # Update breadcrumb
        self.query_one("#breadcrumb-bar", Label).update(f"NAS > {os.path.dirname(file_path).split('/')[-1].upper()} > {os.path.basename(file_path)}")

    @on(ListView.Selected, "#queue-list")
    def on_queue_selected(self, event: ListView.Selected):
        self.play_track(event.list_view.index)

    def add_to_queue(self, track_data, play_now=True):
        track_item = TrackItem(track_data)
        self.queue.append(track_item)
        self.query_one("#queue-list", ListView).append(track_item)
        # If nothing is playing, start playback
        if play_now and self.current_index == -1:
            self.play_track(len(self.queue) - 1)

    def play_track(self, index):
        if 0 <= index < len(self.queue):
            self.current_index = index
            track_item = self.queue[index]
            
            # If local file, check if it exists
            if isinstance(track_item.track_data, str) and not os.path.exists(track_item.track_data):
                self.action_next_track()
                return

            self.player.play(track_item.source)
            # Highlight in list
            queue_list = self.query_one("#queue-list", ListView)
            queue_list.index = index
            
            # Update breadcrumb
            if isinstance(track_item.track_data, str):
                self.query_one("#breadcrumb-bar", Label).update(f"NAS > {os.path.basename(track_item.source)}")
            else:
                self.query_one("#breadcrumb-bar", Label).update(f"STREAMING > YT MUSIC > {track_item.title.upper()}")

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
        artist = self.player.artist
        self.query_one("#track-title", Label).update(title)
        self.query_one("#track-artist", Label).update(artist)

    def update_progress(self, time_pos):
        if time_pos is None: return
        duration = self.player.duration
        if duration:
            percent = (time_pos / duration) * 100
            time_str = f"{self.format_time(time_pos)} / {self.format_time(duration)}"
            now_playing = self.query_one(NowPlaying)
            # Use player.player.pause to determine if playing (mpv property)
            is_playing = not self.player.player.pause
            now_playing.update_info(self.player.title, self.player.artist, percent, time_str, is_playing)
            
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
