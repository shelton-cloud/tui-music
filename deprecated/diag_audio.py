import json
import os
import time

import mpv


def test_playback():
    print("Initializing MPV with updated backend settings...")
    try:
        # Autodetect AO
        player = mpv.MPV(ytdl=True, video=False, terminal=False)
        print(f"Initialization successful. AO: {player.ao}")

        print("Available Audio Devices:")
        devices = player.audio_device_list
        for dev in devices:
            active = "*" if dev['name'] == player.audio_device else " "
            print(f" {active} {dev['description']} ({dev['name']})")

        print(f"Current volume: {player.volume}")
        print(f"Mute status: {player.mute}")

        # Check if we have a song to test with
        test_file = None
        if os.path.exists("songs.json"):
            with open("songs.json", "r", encoding="utf-8") as f:
                songs = json.load(f)
                if songs:
                    test_file = songs[0]

        if not test_file:
            # Try a known stream if no local file
            test_file = "https://www.youtube.com/watch?v=dQw4w9WgXcQ"

        print("Testing YouTube Stream...")
        test_file = "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
        print(f"Attempting to play: {test_file}")
        player.play(test_file)

        # Wait a few seconds for it to start
        for _ in range(10):
            time.sleep(1)
            print(f"Time: {player.time_pos} / {player.duration} | Volume: {player.volume} | Mute: {player.mute}")
            if player.time_pos and player.time_pos > 0:
                print("Playback detected!")
                break
        else:
            print("No playback detected after 10 seconds.")

    except Exception as e:  # pylint: disable=broad-exception-caught
        print(f"Error during playback test: {e}")


if __name__ == "__main__":
    test_playback()
