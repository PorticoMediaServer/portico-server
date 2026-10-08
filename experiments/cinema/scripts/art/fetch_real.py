"""Fetch real artwork from Wikimedia Commons into cache/real/ (idempotent).

- Posters for the public-domain films and Blender open movies in content/real.json.
- Stills from each film's video (Commons renders a frame at any seek time).
- Headshots for real people with a Commons portrait.
- Anonymous public-domain portraits for invented people (cache/portraits/).
"""
import json, os, sys, time, urllib.parse
from commons import get, search, download

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, '..', '..'))
CACHE = os.path.join(HERE, 'cache', 'real')
os.makedirs(CACHE, exist_ok=True)
real = json.load(open(os.path.join(ROOT, 'scripts/catalog/content/real.json')))
STILL_FRACTIONS = [0.08, 0.16, 0.24, 0.32, 0.42, 0.52, 0.62, 0.72, 0.82, 0.9]


def info(title, width=None, param=None):
    params = {'action': 'query', 'titles': title, 'prop': 'imageinfo|videoinfo', 'iiprop': 'url|size|metadata'}
    if width:
        params['iiurlwidth'] = width
    if param:
        params['iiurlparam'] = param
    d = get(params)
    pages = list((d.get('query') or {}).get('pages', {}).values())
    if not pages or 'imageinfo' not in pages[0]:
        return None
    return pages[0]['imageinfo'][0]


def resolve(spec, want='image'):
    if spec is None:
        return None
    if spec.startswith('File:'):
        return spec
    term = spec[len('search:'):]
    for r in search(term, 10):
        t = r['title'].lower()
        if want == 'video' and t.endswith(('.webm', '.ogv')):
            return r['title']
        if want == 'image' and t.endswith(('.jpg', '.jpeg', '.png')) and r['h'] and r['w'] and r['h'] > r['w'] * 1.15:
            return r['title']
    return None


def duration_of(ii):
    md = {m['name']: m['value'] for m in (ii.get('metadata') or [])}
    for key in ('playtime_seconds', 'length', 'duration'):
        if key in md:
            try:
                return float(md[key])
            except (TypeError, ValueError):
                pass
    return None


def fetch_movie(m):
    out = os.path.join(CACHE, m['id'])
    os.makedirs(out, exist_ok=True)
    poster = os.path.join(out, 'poster.jpg')
    if not os.path.exists(poster):
        title = resolve(m['poster'], 'image')
        ii = info(title, 800) if title else None
        if ii and ii.get('thumburl'):
            download(ii['thumburl'], poster)
            print('poster', m['id'], title)
        else:
            print('NO POSTER', m['id'])
    stills = [os.path.join(out, f'still-{i}.jpg') for i in range(len(STILL_FRACTIONS))]
    if all(os.path.exists(s) for s in stills):
        return
    vtitle = resolve(m['video'], 'video')
    if not vtitle:
        print('NO VIDEO', m['id'])
        return
    ii = info(vtitle)
    dur = duration_of(ii or {}) or (m['runtime'] * 60)
    for i, frac in enumerate(STILL_FRACTIONS):
        if os.path.exists(stills[i]):
            continue
        seek = int(dur * frac)
        vi = info(vtitle, 1280, f'1280px-seek={seek}')
        if vi and vi.get('thumburl'):
            ok = download(vi['thumburl'], stills[i])
            print('still', m['id'], seek, ok)
            time.sleep(0.4)


def fetch_person(p):
    path = os.path.join(CACHE, 'people', p['id'] + '.jpg')
    os.makedirs(os.path.dirname(path), exist_ok=True)
    if os.path.exists(path) or not p.get('search'):
        return
    for r in search(p['search'], 10):
        t = r['title'].lower()
        if not t.endswith(('.jpg', '.jpeg', '.png')) or not r['w'] or not r['h']:
            continue
        if r['license'] and 'public domain' not in r['license'].lower() and not r['license'].upper().startswith(('CC BY', 'CC0')):
            continue
        if r['h'] < r['w'] * 0.9:
            continue
        ii = info(r['title'], 500)
        if ii and ii.get('thumburl') and download(ii['thumburl'], path):
            print('person', p['id'], r['title'], r['license'])
            return
    print('NO PERSON', p['id'])


def fetch_portraits(n=160):
    """Anonymous public-domain studio portraits for invented people."""
    out = os.path.join(HERE, 'cache', 'portraits')
    os.makedirs(out, exist_ok=True)
    have = len([f for f in os.listdir(out) if f.endswith('.jpg')])
    if have >= n:
        return
    cats = ['Category:Portrait photographs of unidentified women', 'Category:Portrait photographs of unidentified men',
            'Category:Studio portrait photographs of women', 'Category:Studio portrait photographs of men',
            'Category:Black and white portrait photographs of women', 'Category:Black and white portrait photographs of men']
    count = have
    for cat in cats:
        cont = None
        while count < n:
            params = {'action': 'query', 'generator': 'categorymembers', 'gcmtitle': cat, 'gcmtype': 'file', 'gcmlimit': 50,
                      'prop': 'imageinfo', 'iiprop': 'url|size|extmetadata', 'iiurlwidth': 400}
            if cont:
                params['gcmcontinue'] = cont
            d = get(params)
            for pg in (d.get('query') or {}).get('pages', {}).values():
                ii = (pg.get('imageinfo') or [{}])[0]
                lic = (ii.get('extmetadata', {}).get('LicenseShortName', {}) or {}).get('value', '')
                if 'public domain' not in lic.lower() and not lic.upper().startswith('CC0'):
                    continue
                if not ii.get('thumburl') or not ii.get('width') or ii['height'] < ii['width']:
                    continue
                name = f'portrait-{count:03d}.jpg'
                if download(ii['thumburl'], os.path.join(out, name)):
                    count += 1
                if count >= n:
                    break
            cont = (d.get('continue') or {}).get('gcmcontinue')
            if not cont:
                break
    print('portraits', count)


if __name__ == '__main__':
    what = sys.argv[1] if len(sys.argv) > 1 else 'all'
    if what in ('all', 'movies'):
        for m in real['movies']:
            fetch_movie(m)
    if what in ('all', 'people'):
        for p in real['people']:
            fetch_person(p)
    if what in ('all', 'portraits'):
        fetch_portraits()
