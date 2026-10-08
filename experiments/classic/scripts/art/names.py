#!/usr/bin/env python3
"""Gives the sample music library believable, distinct names.

catalog.py drew album titles from a pool of fifty (so the same title turned up
on several artists) and built every song title as adjective + plural noun.
This rewrites both in src/data/fixtures/catalogData.json and .art-cache/plan.json
(ids stay the same), so no album title repeats and song titles vary the way
real ones do. Re-render the album covers afterwards, since they print the title:
    python3 scripts/art/names.py && python3 scripts/art/render.py albums
"""
import json, os, random

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
DATA = os.path.join(ROOT, 'src', 'data', 'fixtures', 'catalogData.json')
PLAN = os.path.join(ROOT, '.art-cache', 'plan.json')

COUNT = """lighthouse satellite harbor river orchard meadow highway window doorway garden porch
 radio telephone letter postcard photograph ribbon feather sparrow swallow heron fox wolf deer horse
 lion whale island mountain valley canyon city streetlight station ferry bridge tunnel rooftop
 hallway ember lantern compass anchor candle match spark star moon comet planet galaxy heartbeat
 shadow echo mirror carousel parade holiday birthday wedding promise secret rumor story chapter song
 melody drum guitar piano violin storm kite paper-plane""".replace('paper-plane', 'airplane').split()
MASS = """undertow midnight morning winter summer evening daylight weather thunder fire smoke ash
 stone iron copper silver gold glass paper cotton velvet linen salt honey sugar rain snow frost
 fog mist wind breeze lightning sunlight orbit harmony static""".split()
SOFT = """weather thunder rain snow frost fog mist wind fire smoke daylight morning evening midnight
 summer winter harmony static sunlight""".split()
NOUNS = COUNT + MASS
ADJ = """quiet golden hollow gentle restless electric wild patient careful distant tender
 northern southern western eastern broken silver faded borrowed stolen secret lonely
 crooked bright heavy weightless slow sudden open empty crowded blue green scarlet amber
 violet pale late early first last little hidden sleeping burning falling rising wandering fading
 shining hungry honest foolish lucky lazy painted secondhand homemade bitter sweet dusty rusty frozen
 sleepless endless nameless careless reckless crimson ivory indigo cobalt wooden neon faraway holy
 ordinary perfect tiny giant""".split()
PLACES = """August September October November December January February March April May June July
 Sunday Monday Tuesday Wednesday Thursday Friday Saturday Memphis Portland Tallahassee Denver
 Savannah Tucson Boise Galveston Baltimore Duluth Asheville""".split()
NAMES = """Josephine Carolina Eleanor Marisol Delilah Rosalind Theodore August Evangeline Margaret
 Lucinda Harriet Winona Juniper Clementine Abigail Samuel Isadora Magdalena Ophelia Vivian Beatrice
 Henrietta Loretta Rosemary Cassidy Everett Jonah Silas Walter Florence Cecilia Maribel Lorena Sadie
 Hazel Mabel Otis Ruby Stella Frances""".split()
PHRASES = """Take the Long Way|Still Here|Home Before Dark|Tell Me Twice|If the River Rises|Hold the Line
|Carry the Light|Chasing Weather|Let It Ring|Nothing to Lose|All the Way Down|Somewhere Near|Close Enough
|Call Me When You Land|Keep the Porch Light On|Out of Season|Back to the Water|Better Days|No Small Thing
|Leave the Door Open|Slow Down|Run for Cover|What We Kept|After the Rain|Before the Morning|Almost Home
|Every Little Thing|One More Mile|Stay a While|Say It Plain|Half a World Away|Where the Road Bends
|Heart of the Matter|Nowhere Fast|Counting Backwards|Found Out|Fall Apart Gently|Sleep It Off
|Wake Up Easy|On the Other Side|Under the Wire|In the Meantime|By the Water|For Now|For Keeps
|Hand in Hand|Head Over Heels|Out Loud|Over and Over|Right on Time|Long Story Short|Like Clockwork
|Two Steps Back|Three Chords|Four Walls|Five Minutes More|Seven Years|Ninety Nine|A Hundred Miles
|Just Like That|All I Need|Holding Pattern|Open Water|Wide Awake|Falling Upward|Turn It Around
|Word for Word|Rise and Shine|Easy Come|Last Call|Paper Thin|Glass Half Full|Moving Day|Snow Day
|Road Trip|Night Drive|Slow Dance|Fast Car|Blue Jeans|Cold Coffee|Warm Hands|Sweet Talk|Small Talk
|Pocket Change|Loose Ends|Dead Air|Clean Slate|Fresh Paint|New Shoes|Old Friends|True North
|Lost and Found|Hide and Seek|Rock Paper Scissors|Heads or Tails|Pins and Needles|Salt and Pepper""".replace('\n', '').split('|')
VERB_PHRASES = """Carry the Light|Follow the River|Chase the Weather|Hold the Line|Keep the Lights On|Leave the Light On
|Find the Road|Lose the Map|Burn the Letters|Build a Fire|Break the Silence|Mend the Fence|Cross the River|Climb the Stairs
|Wake the Neighbors|Call It Home|Tell the Truth|Remember the Night|Forget the Rest|Paint the Town|Sing It Back|Hum Along
|Whistle Past|Dance It Off|Wait It Out|Watch the Sky|Ring the Bells|Raise the Flag|Turn the Wheel|Pull the Rope
|Open the Window|Close the Door|Start the Car|Change the Station|Read the Letter|Write It Down|Say My Name|Mind the Gap
|Follow the Sun|Count the Stars|Beat the Clock|Catch the Wind|Hold Your Fire|Walk the Line|Name the Stars|Fly the Kite
|Steady the Boat|Tune the Radio|Cut the Cord|Light the Lanterns|Save the Date|Miss the Train|Meet Me There|Row the Boat""".replace('\n', '').split('|')


SUFFIX = ['Song', 'Blues', 'Waltz', 'Lullaby', 'Hymn', 'Serenade', 'Reprise', 'Theme']
first_words = {}


def plural(n):
    irregular = {'wolf': 'wolves', 'deer': 'deer', 'echo': 'echoes', 'story': 'stories', 'ferry': 'ferries', 'city': 'cities', 'valley': 'valleys', 'holiday': 'holidays', 'birthday': 'birthdays', 'highway': 'highways', 'doorway': 'doorways', 'hallway': 'hallways', 'galaxy': 'galaxies', 'melody': 'melodies'}
    if n in irregular:
        return irregular[n]
    return n + ('es' if n.endswith(('s', 'x', 'ch', 'sh')) else 's')


def title(r, used, repeat=1, avoid=()):
    """A plausible song or album title: curated phrases, plus a few patterns
    whose first words are capped so no pattern shows through in an A to Z list."""
    for attempt in range(6000):
        if attempt == 3000:
            repeat = max(repeat, 2)  # vocabulary exhausted: let a title recur on another album
        k = r.random()
        n1 = r.choice(NOUNS)
        c1 = r.choice(COUNT)
        if k < 0.24:
            t = r.choice(PHRASES)
        elif k < 0.42:
            t = plural(c1).capitalize() if r.random() < 0.35 else n1.capitalize()
        elif k < 0.66:
            t = f'{r.choice(ADJ).capitalize()} {(plural(c1) if r.random() < 0.3 else c1).capitalize()}' if r.random() < 0.75 else f'{r.choice(ADJ).capitalize()} {r.choice(SOFT).capitalize()}'
        elif k < 0.74:
            t = r.choice(VERB_PHRASES)
        elif k < 0.82:
            t = f'{r.choice(PLACES)} {r.choice(["Rain", "Nights", "Morning", "Skies", "Light", "Blues", "Moon", "Wind"])}'
        elif k < 0.90:
            t = f'{n1.capitalize()} {r.choice(SUFFIX)}'
        elif k < 0.93:
            t = f'The {(plural(c1) if r.random() < 0.4 else c1).capitalize()}'
        elif k < 0.97:
            t = f'Ballad of the {c1.capitalize()}'
        else:
            nm = r.choice(NAMES)
            t = r.choice([nm, f'{nm}, Come Home', f'Letter to {nm}', f'Song for {nm}', f'{nm} in the Rain'])
        w = t.split()[0] if not t.startswith('The ') else 'The ' + t.split()[1]
        adj = {x.capitalize() for x in ADJ}
        cap = (8 if w == 'Ballad' else 6 if w in adj else 2 if w in PLACES else 5 if w in ('Letter', 'Song') else 3) * repeat
        if used.get(t, 0) >= repeat or t in avoid or first_words.get(w, 0) >= cap:
            continue
        first_words[w] = first_words.get(w, 0) + 1
        used[t] = used.get(t, 0) + 1
        return t
    raise SystemExit('ran out of names')


def rewrite(doc, seed):
    r = random.Random(seed)
    # Albums: keep each title the first time it appears, rename the repeats.
    # Songs may share a title across artists (at most twice), never within an album.
    album_used, songs_used = {}, {}
    for a in doc['albums']:
        if a['title'] in album_used:
            a['title'] = title(r, album_used)
        else:
            album_used[a['title']] = 1
    for a in doc['albums']:
        here = set()
        for t in a['tracks']:
            t['title'] = title(r, songs_used, 1, here)
            here.add(t['title'])
    return doc


data = rewrite(json.load(open(DATA)), 7)
json.dump(data, open(DATA, 'w'), ensure_ascii=False, separators=(',', ':'))
if os.path.exists(PLAN):
    plan = json.load(open(PLAN))
    names = {a['id']: (a['title'], {t['id']: t['title'] for t in a['tracks']}) for a in data['albums']}
    for a in plan['albums']:
        if a['id'] in names:
            a['title'] = names[a['id']][0]
            for t in a.get('tracks', []):
                t['title'] = names[a['id']][1].get(t['id'], t['title'])
    json.dump(plan, open(PLAN, 'w'))
print(len(data['albums']), 'albums,', sum(len(a['tracks']) for a in data['albums']), 'songs renamed where needed')
