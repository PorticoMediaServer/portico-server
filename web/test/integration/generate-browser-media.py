#!/usr/bin/env python3
"""Small synthetic local-only browser library. Run with an empty scratch directory."""
import pathlib, subprocess, sys
from xml.sax.saxutils import escape
root = pathlib.Path(sys.argv[1]).resolve()
def run(args):
    subprocess.run(["ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-threads", "1", "-y", *args], check=True)
def video(rel, title, extra=""):
    file = root / rel
    file.parent.mkdir(parents=True, exist_ok=True)
    run(["-f", "lavfi", "-i", "testsrc2=size=640x360:rate=24:duration=45", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=45", "-c:v", "libx264", "-threads", "1", "-preset", "ultrafast", "-crf", "28", "-pix_fmt", "yuv420p", "-c:a", "aac", "-movflags", "+faststart", "-shortest", "-metadata", "title="+title, str(file)])
    file.with_suffix(".nfo").write_text(f"<movie><title>{escape(title)}</title><year>2024</year><genre>Testing</genre><plot>Synthetic browser test media.</plot>{extra}</movie>")
    return file
film = video("movies/Browser Film (2024)/Browser Film (2024).mp4", "Browser Film")
video("movies/Second Film (2024)/Second Film (2024).mp4", "Second Film")
video("tv/Browser Show/Season 1/Browser Show S01E01.mp4", "First Episode", "<season>1</season><episode>1</episode><showtitle>Browser Show</showtitle>")
video("tv/Browser Show/Season 1/Browser Show S01E02.mp4", "Second Episode", "<season>1</season><episode>2</episode><showtitle>Browser Show</showtitle>")
video("anime/Browser Anime/Season 1/Browser Anime S01E01.mp4", "Anime Episode", "<season>1</season><episode>1</episode><showtitle>Browser Anime</showtitle>")
film.with_suffix(".en.srt").write_text("1\n00:00:00,000 --> 00:00:40,000\nBrowser text subtitle\n")
film.with_suffix(".en.ass").write_text("""[Script Info]
ScriptType: v4.00+
PlayResX: 640
PlayResY: 360
[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding
Style: Default,Arial,28,&H0000FFFF,&H000000FF,&H00000000,&H80000000,0,0,0,0,100,100,0,0,1,2,0,2,10,10,24,1
[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:00.00,0:00:40.00,Default,,0,0,0,,Browser styled subtitle
""")
for kind, rel in [("music", "Browser Artist/Browser Album/01 - Browser Song.flac"), ("audiobook", "Browser Author/Browser Book/01 - Chapter One.m4b")]:
    file = root / kind / rel
    file.parent.mkdir(parents=True, exist_ok=True)
    run(["-f", "lavfi", "-i", "sine=frequency=330:sample_rate=44100:duration=45", "-c:a", "flac" if kind=="music" else "aac", "-metadata", "artist=Browser Artist" if kind=="music" else "artist=Browser Author", "-metadata", "album=Browser Album" if kind=="music" else "album=Browser Book", "-metadata", "title=Browser Song" if kind=="music" else "title=Chapter One", "-metadata", "track=1", str(file)])
print(root)
