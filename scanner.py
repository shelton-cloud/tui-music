import os
import json
from mutagen.easyid3 import EasyID3
from mutagen.flac import FLAC
from mutagen.mp4 import MP4
from mutagen.oggvorbis import OggVorbis

def get_metadata(file_path):
    """Extract artist, title, and album using mutagen."""
    metadata = {
        "path": file_path,
        "title": os.path.basename(file_path),
        "artist": "Unknown Artist",
        "album": "Unknown Album"
    }
    
    try:
        ext = os.path.splitext(file_path)[1].lower()
        audio = None
        
        if ext == '.mp3':
            try:
                audio = EasyID3(file_path)
            except:
                # Fallback for MP3s without ID3 tags
                pass
        elif ext == '.flac':
            audio = FLAC(file_path)
        elif ext in ('.m4a', '.mp4'):
            audio = MP4(file_path)
        elif ext == '.ogg':
            audio = OggVorbis(file_path)

        if audio:
            # mutagen EasyID3 and others return lists for values
            metadata["title"] = audio.get("title", [metadata["title"]])[0]
            metadata["artist"] = audio.get("artist", ["Unknown Artist"])[0]
            metadata["album"] = audio.get("album", ["Unknown Album"])[0]
            
    except Exception as e:
        # If tag reading fails, we still have the filename as title
        pass
        
    return metadata

def scan_library(root_path, limit=None):
    songs = []
    extensions = ('.mp3', '.flac', '.wav', '.m4a', '.ogg')
    
    print(f"Scanning {root_path} for music files with rich metadata...")
    
    count = 0
    for root, dirs, files in os.walk(root_path):
        for file in files:
            if file.lower().endswith(extensions):
                full_path = os.path.join(root, file)
                songs.append(get_metadata(full_path))
                count += 1
                if count % 10 == 0:
                    print(f"Indexed {count} songs...")
                if limit and count >= limit:
                    return songs
    
    return songs

if __name__ == "__main__":
    import sys
    limit = int(sys.argv[1]) if len(sys.argv) > 1 else None
    mount_path = "/Volumes/music"
    if not os.path.exists(mount_path):
        print(f"Error: {mount_path} not found. Please ensure your NAS is mounted.")
        # Fallback for testing if NASA is unmounted
        mount_path = "./" 
        
    library = scan_library(mount_path, limit=limit)
    
    with open("songs.json", "w") as f:
        json.dump(library, f, indent=4)
        
    print(f"Successfully indexed {len(library)} songs into songs.json")
