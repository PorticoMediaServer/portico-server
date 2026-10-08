#!/usr/bin/env python3
"""Portico public demo: Live TV fixture generator.

Writes channels.m3u, guide.xml and logos/*.svg under /srv/portico-demo/live.
Every name is fictional and every stream is the copyright-safe demo loop, so the
fixture exists only to exercise guide and channel UI: varied programme lengths,
episode metadata, ratings, categories, new/live/premiere flags, a channel with no
guide data, a channel with gaps, and a channel with very long titles.
Deterministic: the same UTC day produces the same schedule (seeded per channel and day).
"""
import datetime as dt, hashlib, html, os, random

ROOT = os.environ.get("FIXTURE_ROOT", "/srv/portico-demo/live")
BASE = "https://demo.getportico.tv/fixtures/live"
# Endless sliding-window HLS from the host's synthetic generator (portico-synthetic-live),
# published by nginx under /fixtures/live/streams/. Channels rotate over the three feeds.
STREAMS = [BASE + "/streams/" + name + "/index.m3u8" for name in ("demo-cinema", "demo-family", "demo-music")]
DAYS_BACK, DAYS_AHEAD = 1, 7

CHANNELS = [
    # id, number, name, group, colour, style
    ("north.news", 101, "Northwind News", "News", "#c0392b", "news"),
    ("north.news2", 102, "Northwind News 24", "News", "#a93226", "news"),
    ("summit.sports", 201, "Summit Sports", "Sports", "#1e8449", "sports"),
    ("summit.sports2", 202, "Summit Sports Extra", "Sports", "#196f3d", "sports"),
    ("lantern.movies", 301, "Lantern Movies", "Movies", "#7d3c98", "movies"),
    ("lantern.classics", 302, "Lantern Classics", "Movies", "#6c3483", "movies"),
    ("pebble.kids", 401, "Pebble Kids", "Kids", "#f39c12", "kids"),
    ("atlas.docs", 501, "Atlas Documentaries", "Documentary", "#2874a6", "docs"),
    ("tidal.music", 601, "Tidal Music TV", "Music", "#17a589", "music"),
    ("harbor.local", 701, "Harbor City Local", "Local", "#5d6d7e", "local"),
    ("comet.comedy", 801, "Comet Comedy", "Entertainment", "#d35400", "series"),
    ("ember.drama", 802, "Ember Drama", "Entertainment", "#922b21", "series"),
    ("quiet.noguide", 901, "Quiet Channel (no guide)", "Other", "#34495e", "none"),
    ("patchy.gaps", 902, "Patchwork TV (guide gaps)", "Other", "#7f8c8d", "gaps"),
]

SERIES = {
    "news": [("Morning Briefing", 60), ("Midday Report", 30), ("World Tonight", 60), ("Market Watch", 30), ("Weather Now", 15), ("Late Edition", 30)],
    "sports": [("Summit Football: Harbor City vs. Ridgeview", 150), ("Match Highlights", 30), ("The Locker Room", 60), ("Cycling: Mountain Stage", 180), ("Tennis Open: Quarterfinal", 120)],
    "movies": [("The Lighthouse Keeper's Daughter", 118), ("Paper Moons", 96), ("Beneath the Glass Sky", 134), ("A Quiet Harbor", 102), ("The Long Road North", 147), ("Midnight at the Observatory", 109)],
    "kids": [("Pebble and Friends", 15), ("Rocket Raccoons", 25), ("Draw Along", 15), ("The Tiny Gardeners", 25), ("Story Time Island", 30)],
    "docs": [("Oceans Unseen", 60), ("Cities That Float", 50), ("The Clockmakers", 45), ("Wild Plains", 60), ("How Bridges Stand", 30)],
    "music": [("Top 20 Countdown", 60), ("Acoustic Sessions", 30), ("Live from the Harbor Stage", 90), ("Throwback Mix", 60)],
    "local": [("Harbor City This Morning", 90), ("Council Live", 120), ("Community Kitchen", 30), ("Local Sports Roundup", 30)],
    "series": [("The Night Shift", 44), ("Neighbors & Rivals", 22), ("Station Nine", 44), ("Half Past Seven", 22), ("The Inheritance: A Very Long Title That Should Truncate Gracefully In Every Guide Cell", 44)],
}
EPISODE_NAMES = ["The Arrival", "Crossed Wires", "Low Tide", "Second Chances", "Open Doors", "The Long Night", "Borrowed Time", "Homecoming", "Fault Lines", "First Light"]
RATINGS = {"kids": "TV-Y", "movies": "PG-13", "series": "TV-14", "news": "TV-G", "sports": "TV-G", "docs": "TV-PG", "music": "TV-PG", "local": "TV-G"}
CATEGORIES = {"news": ["News"], "sports": ["Sports", "Live"], "movies": ["Movie", "Drama"], "kids": ["Kids", "Animation"], "docs": ["Documentary", "Nature"], "music": ["Music"], "local": ["Local", "Talk"], "series": ["Series", "Comedy"]}


def fmt(t: dt.datetime) -> str:
    return t.strftime("%Y%m%d%H%M%S +0000")


def logo_svg(name: str, colour: str) -> str:
    letters = "".join(w[0] for w in name.split()[:2]).upper()
    return (f'<svg xmlns="http://www.w3.org/2000/svg" width="256" height="256" viewBox="0 0 256 256">'
            f'<rect width="256" height="256" rx="40" fill="{colour}"/>'
            f'<text x="128" y="160" font-family="Helvetica, Arial, sans-serif" font-size="104" font-weight="700" '
            f'text-anchor="middle" fill="#ffffff">{html.escape(letters)}</text></svg>')


def schedule(ch_id: str, style: str, day: dt.date):
    rng = random.Random(hashlib.sha256(f"{ch_id}:{day.isoformat()}".encode()).digest())
    start = dt.datetime.combine(day, dt.time(0, 0), tzinfo=dt.timezone.utc)
    end = start + dt.timedelta(days=1)
    t, out = start, []
    pool = SERIES["series" if style == "gaps" else style]
    while t < end:
        title, minutes = rng.choice(pool)
        # Snap to the half hour for most styles so the grid looks like real TV.
        slot = minutes if style in ("movies", "sports") else max(15, round(minutes / 15) * 15)
        stop = min(end, t + dt.timedelta(minutes=slot))
        if style == "gaps" and rng.random() < 0.25:
            t = stop  # leave a hole in the guide
            continue
        out.append((t, stop, title))
        t = stop
    return out, rng


def main():
    os.makedirs(os.path.join(ROOT, "logos"), exist_ok=True)
    today = dt.datetime.now(dt.timezone.utc).date()
    m3u = ["#EXTM3U"]
    for i, (ch_id, num, name, group, colour, style) in enumerate(CHANNELS):
        with open(os.path.join(ROOT, "logos", ch_id + ".svg"), "w") as f:
            f.write(logo_svg(name, colour))
        m3u.append(f'#EXTINF:-1 tvg-id="{ch_id}" tvg-chno="{num}" tvg-name="{html.escape(name)}" '
                   f'tvg-logo="{BASE}/logos/{ch_id}.svg" group-title="{group}",{name}')
        m3u.append(STREAMS[i % len(STREAMS)])
    xml = ['<?xml version="1.0" encoding="UTF-8"?>', '<tv generator-info-name="Portico Public Demo">']
    for ch_id, num, name, group, colour, style in CHANNELS:
        xml += [f'  <channel id="{ch_id}">', f'    <display-name>{html.escape(name)}</display-name>',
                f'    <display-name>{num}</display-name>', f'    <icon src="{BASE}/logos/{ch_id}.svg"/>', '  </channel>']
    for ch_id, num, name, group, colour, style in CHANNELS:
        if style == "none":
            continue
        for offset in range(-DAYS_BACK, DAYS_AHEAD + 1):
            day = today + dt.timedelta(days=offset)
            progs, rng = schedule(ch_id, style, day)
            for start, stop, title in progs:
                kind = "series" if style == "gaps" else style
                xml.append(f'  <programme start="{fmt(start)}" stop="{fmt(stop)}" channel="{ch_id}">')
                xml.append(f'    <title lang="en">{html.escape(title)}</title>')
                episodic = kind in ("series", "kids", "docs", "local")
                if episodic:
                    season, episode = rng.randint(1, 6), rng.randint(1, 12)
                    xml.append(f'    <sub-title lang="en">{html.escape(rng.choice(EPISODE_NAMES))}</sub-title>')
                xml.append(f'    <desc lang="en">{html.escape(f"{title} on {name}. A fictional listing from the Portico demo guide, used to test how the guide shows descriptions of different lengths.")}</desc>')
                for c in CATEGORIES.get(kind, ["Other"]):
                    xml.append(f'    <category lang="en">{c}</category>')
                if episodic:
                    xml.append(f'    <episode-num system="xmltv_ns">{season - 1}.{episode - 1}.</episode-num>')
                    xml.append(f'    <episode-num system="onscreen">S{season:02d}E{episode:02d}</episode-num>')
                if kind == "movies":
                    xml.append(f'    <date>{rng.randint(1958, 2024)}</date>')
                    xml.append(f'    <star-rating><value>{rng.randint(5, 9)}/10</value></star-rating>')
                if kind == "sports" and "vs." in title or kind == "news" and title == "World Tonight":
                    xml.append('    <live/>')
                elif episodic and rng.random() < 0.2:
                    xml.append('    <new/>')
                elif kind == "movies" and rng.random() < 0.1:
                    xml.append('    <premiere/>')
                xml.append(f'    <rating system="US"><value>{RATINGS.get(kind, "TV-G")}</value></rating>')
                xml.append('  </programme>')
    xml.append('</tv>')
    for fname, body in (("channels.m3u", "\n".join(m3u) + "\n"), ("guide.xml", "\n".join(xml) + "\n")):
        tmp = os.path.join(ROOT, fname + ".tmp")
        with open(tmp, "w") as f:
            f.write(body)
        os.replace(tmp, os.path.join(ROOT, fname))


if __name__ == "__main__":
    main()
