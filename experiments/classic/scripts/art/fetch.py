#!/usr/bin/env python3
"""
Downloads source imagery into .art-cache/src (not committed):
  - Wikimedia Commons film posters, stills and people photos (public domain / CC BY),
    looked up through the Wikipedia and Commons APIs. Files hosted outside Commons
    (local fair-use uploads) are skipped.
  - Unsplash photos through picsum.photos seeds.
Idempotent: existing files are kept. Run after catalog.py.
"""
import json, os, ssl, sys, time, urllib.parse, urllib.request
CTX = ssl.create_default_context(cafile='/etc/ssl/cert.pem')
from concurrent.futures import ThreadPoolExecutor

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
CACHE = os.path.join(ROOT, '.art-cache')
SRC = os.path.join(CACHE, 'src')
os.makedirs(SRC, exist_ok=True)
UA = 'PorticoClassicFixtureArt/1.0 (personal prototype; contact justinehler@icloud.com)'
plan = json.load(open(os.path.join(CACHE, 'plan.json')))
credits = {}
credits_path = os.path.join(CACHE, 'credits.json')
if os.path.exists(credits_path): credits = json.load(open(credits_path))

def get(url, retries=4):
    for i in range(retries):
        try:
            req = urllib.request.Request(url, headers={'User-Agent': UA})
            with urllib.request.urlopen(req, timeout=40, context=CTX) as r:
                return r.read()
        except Exception as e:
            if i == retries - 1:
                print('FAIL', url, e, file=sys.stderr)
                return None
            time.sleep(1.5 * (i + 1))

def api(base, params):
    params = dict(params, format='json', formatversion='2')
    data = get(base + '?' + urllib.parse.urlencode(params))
    return json.loads(data) if data else {}

def save(path, url):
    if os.path.exists(path) and os.path.getsize(path) > 2000: return True
    data = get(url)
    if not data or len(data) < 2000: return False
    open(path, 'wb').write(data)
    return True

WIKI = 'https://en.wikipedia.org/w/api.php'
COMMONS = 'https://commons.wikimedia.org/w/api.php'

def page_images(titles, size):
    out = {}
    for i in range(0, len(titles), 40):
        chunk = titles[i:i + 40]
        res = api(WIKI, {'action': 'query', 'titles': '|'.join(chunk), 'prop': 'pageimages', 'piprop': 'thumbnail|name', 'pithumbsize': size, 'redirects': 1})
        q = res.get('query', {})
        norm = {n['from']: n['to'] for n in q.get('normalized', [])}
        redir = {n['from']: n['to'] for n in q.get('redirects', [])}
        pages = {p['title']: p for p in q.get('pages', [])}
        for t in chunk:
            tt = redir.get(norm.get(t, t), norm.get(t, t))
            p = pages.get(tt)
            if p and 'thumbnail' in p:
                out[t] = (p['thumbnail']['source'], p.get('pageimage'))
        time.sleep(0.3)
    return out

def commons_only(url):
    return '/wikipedia/commons/' in url

# 1. Film posters
films = [m for m in plan['movies'] if m['art']['kind'] == 'wiki']
posters = page_images([m['art']['article'] for m in films], 600)
wiki_ok = {}
for m in films:
    hit = posters.get(m['art']['article'])
    if hit and commons_only(hit[0]):
        path = os.path.join(SRC, f"wposter-{m['id']}.jpg")
        if save(path, hit[0]):
            wiki_ok[m['id']] = True
            credits[m['id'] + ':poster'] = {'source': 'Wikimedia Commons', 'file': hit[1]}
print('film posters from Commons:', len(wiki_ok), 'of', len(films))

# 2. Film stills from Commons categories
def category_still(cat, out_path):
    if os.path.exists(out_path): return True
    res = api(COMMONS, {'action': 'query', 'generator': 'categorymembers', 'gcmtitle': 'Category:' + cat, 'gcmtype': 'file', 'gcmlimit': 40, 'prop': 'imageinfo', 'iiprop': 'url|size|mime', 'iiurlwidth': 1280})
    pages = res.get('query', {}).get('pages', [])
    best = None
    for p in pages:
        ii = (p.get('imageinfo') or [{}])[0]
        w, h = ii.get('width', 0), ii.get('height', 1)
        if ii.get('mime') not in ('image/jpeg', 'image/png'): continue
        if w < 700 or w / max(h, 1) < 1.25 or w / max(h, 1) > 2.6: continue
        name = p['title'].lower()
        if any(k in name for k in ('poster', 'lobby', 'title card', 'advert', 'program', 'dvd', 'logo', 'script', 'page')): continue
        score = w
        if best is None or score > best[0]: best = (score, ii.get('thumburl') or ii.get('url'), p['title'])
    if best and save(out_path, best[1]):
        return best[2]
    return None

still_ok = 0
for m in films:
    t = category_still(m['art']['category'], os.path.join(SRC, f"wstill-{m['id']}.jpg"))
    if t:
        still_ok += 1
        if t is not True: credits[m['id'] + ':backdrop'] = {'source': 'Wikimedia Commons', 'file': t}
    time.sleep(0.25)
print('film stills from Commons:', still_ok)

# 3. People photos for real people
real_people = [p for p in plan['people'] if p.get('wiki')]
photos = page_images([p['wiki'] for p in real_people], 400)
pp = 0
for p in real_people:
    hit = photos.get(p['wiki'])
    if hit and commons_only(hit[0]):
        if save(os.path.join(SRC, f"wperson-{p['id']}.jpg"), hit[0]):
            pp += 1
            credits[p['id'] + ':photo'] = {'source': 'Wikimedia Commons', 'file': hit[1]}
print('people photos from Commons:', pp, 'of', len(real_people))
json.dump(credits, open(credits_path, 'w'), indent=1)

# 4. Picsum (Unsplash) photographs
jobs = []
def pic(name, seed, w, h):
    jobs.append((os.path.join(SRC, name), f'https://picsum.photos/seed/{seed}/{w}/{h}'))
for m in plan['movies']:
    if m['art']['kind'] == 'generated': pic(f"pposter-{m['id']}.jpg", 'pc' + m['id'], 400, 600)
    pic(f"pback-{m['id']}.jpg", 'bd' + m['id'], 1280, 720)
for s in plan['shows']:
    pic(f"pposter-{s['id']}.jpg", 'pc' + s['id'], 400, 600)
    pic(f"pback-{s['id']}.jpg", 'bd' + s['id'], 1280, 720)
for n in range(520):
    pic(f"pstill-{n}.jpg", f'still{n}', 480, 270)
for a in plan['albums']:
    pic(f"palbum-{a['id']}.jpg", 'al' + a['id'], 500, 500)
for a in plan['artists']:
    pic(f"partist-{a['id']}.jpg", 'ar' + a['id'], 500, 500)
    pic(f"partistbg-{a['id']}.jpg", 'arb' + a['id'], 1280, 720)
for b in plan['books']:
    pic(f"pbook-{b['id']}.jpg", 'bk' + b['id'], 400, 600)
for al in plan['photoAlbums']:
    for ph in al['photos']:
        w, h = (960, 1280) if ph['portrait'] else (1280, 960)
        pic(f"photo-{ph['id']}.jpg", 'photo' + str(ph['seed']), w, h)
for v in plan['homeVideos']:
    pic(f"phome-{v['id']}.jpg", 'hv' + v['id'], 1280, 720)
for i in range(12):
    pic(f"pauth-{i}.jpg", f'auth{i}', 1280, 720)

def run(job):
    path, url = job
    return save(path, url)
with ThreadPoolExecutor(20) as ex:
    results = list(ex.map(run, jobs))
print('picsum', sum(results), 'of', len(jobs))
