import os
import json

def scan_library(root_path):
    songs = []
    extensions = ('.mp3', '.flac', '.wav', '.m4a', '.ogg')
    
    print(f"Scanning {root_path} for music files...")
    
    for root, dirs, files in os.walk(root_path):
        for file in files:
            if file.lower().endswith(extensions):
                full_path = os.path.join(root, file)
                songs.append(full_path)
    
    return songs

if __name__ == "__main__":
    mount_path = "/Volumes/music"
    if not os.path.exists(mount_path):
        print(f"Error: {mount_path} not found. Please ensure your NAS is mounted.")
        # Fallback for testing if NASA is unmounted
        mount_path = "./" 
        
    library = scan_library(mount_path)
    
    with open("songs.json", "w") as f:
        json.dump(library, f, indent=4)
        
    print(f"Successfully indexed {len(library)} songs into songs.json")
