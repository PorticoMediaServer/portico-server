"""Pack the catalogue content JSON files into one minified module the app bundles.

Reads scripts/catalog/content/*.json and writes src/data/fixtures/content.json.
Missing files become empty collections, so the app always builds.
"""
import json, os

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, '..', '..'))
C = os.path.join(HERE, 'content')


def load(name, default):
    p = os.path.join(C, name)
    if not os.path.exists(p):
        return default
    try:
        with open(p) as f:
            return json.load(f)
    except ValueError as e:
        print('skipping unfinished', name, e)
        return default


real = load('real.json', {'people': [], 'movies': []})
out = {
    'real': real,
    'people': load('people.json', []) + load('people-tv.json', []),
    'movies': load('movies.json', []),
    'shows': load('shows.json', []),
    'audiobooks': load('audiobooks.json', []),
    'music': load('music.json', []),
    'lyrics': load('lyrics.json', {}),
    'live': load('live.json', {'channels': []}),
    'personal': load('personal.json', {'homeVideos': [], 'photoAlbums': []}),
}
dest = os.path.join(ROOT, 'src/data/fixtures/content.json')
with open(dest, 'w') as f:
    json.dump(out, f, ensure_ascii=False, separators=(',', ':'))
print('packed', dest, os.path.getsize(dest), 'bytes;', {k: len(v) if isinstance(v, list) else '' for k, v in out.items()})
