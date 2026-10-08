"""Build every bundled artwork image and the art manifest the app imports.

Reads src/data/fixtures/content.json (run scripts/catalog/pack.py first) and writes:
- assets/art/<category>/<id>.jpg|png
- src/data/fixtures/art.generated.ts  (require() map + dominant tints)

Sources (all freely licensed): Wikimedia Commons public-domain films and Blender open movies
(fetched by fetch_real.py), Unsplash photos via picsum.photos, anonymous public-domain portraits
from Commons, and generated typography. Idempotent: existing outputs are kept unless FORCE=1.
"""
import colorsys, hashlib, json, os, random, subprocess, sys
from PIL import Image, ImageDraw, ImageEnhance, ImageFilter, ImageFont, ImageOps

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, '..', '..'))
CACHE = os.path.join(HERE, 'cache')
PICSUM = os.path.join(CACHE, 'picsum')
OUT = os.path.join(ROOT, 'assets', 'art')
FONTS = os.path.join(CACHE, 'fonts')
FORCE = os.environ.get('FORCE') == '1'
ONLY = set(sys.argv[1:])
os.makedirs(PICSUM, exist_ok=True)

content = json.load(open(os.path.join(ROOT, 'src/data/fixtures/content.json')))

PALETTE = {'teal': (47, 164, 160), 'amber': (217, 154, 58), 'crimson': (194, 66, 79), 'violet': (138, 95, 208), 'emerald': (63, 174, 120),
           'cobalt': (63, 111, 224), 'rose': (212, 102, 143), 'gold': (216, 180, 74), 'slate': (111, 135, 150), 'orange': (224, 118, 58)}
INK = (7, 16, 24)

# Mundane desk/laptop/coffee/food shots and near-blank frames are not cinematic.
EXCLUDE = {0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 20, 24, 26, 30, 36, 39, 42, 48, 60, 63, 119, 160, 175, 180, 201, 252, 305, 312, 326, 357, 366, 367, 370, 425, 429, 431,
           445, 464, 486, 488, 490, 491, 493, 504, 526, 527, 528, 529, 530, 531, 532, 533, 534, 535, 668, 691, 728, 755, 766, 802, 816, 824, 835, 882, 885, 886,
           999, 1009, 1060, 1068, 1080, 857, 1073, 1058, 212, 225, 228, 250, 319, 341, 373, 395, 442, 455, 580, 585, 616, 625, 628, 635, 663, 671, 674, 686, 691,
           757, 785, 811, 833, 834, 858, 894, 887, 921, 951, 954, 1013, 1025, 1032, 1062, 1066, 1070, 1071}
PEOPLE_PHOTOS = [64, 65, 91, 177, 334, 338, 349, 375, 399, 473, 497, 548, 550, 602, 646, 656, 660, 661, 669, 685, 777, 778, 786, 832, 836, 863, 883, 996, 1005, 1011, 1014, 1027, 1001, 823, 839, 453, 158, 451]
ALL_IDS = [int(i['id']) for i in json.load(open(os.path.join(CACHE, 'picsum-list.json')))]
POOL = [i for i in ALL_IDS if i not in EXCLUDE and i not in PEOPLE_PHOTOS]


def h(s):
    return int(hashlib.md5(s.encode()).hexdigest()[:8], 16)


class Allocator:
    """Hands out picsum ids so neighbouring titles rarely share a photo."""
    def __init__(self, pool, seed):
        self.pool = pool[:]
        random.Random(seed).shuffle(self.pool)
        self.i = 0

    def take(self):
        v = self.pool[self.i % len(self.pool)]
        self.i += 1
        return v


def font(name, size):
    return ImageFont.truetype(os.path.join(FONTS, name), size)


def picsum(pid, w, hgt):
    path = os.path.join(PICSUM, f'{pid}_{w}x{hgt}.jpg')
    return path


NEEDED = []


def need(pid, w, hgt):
    p = picsum(pid, w, hgt)
    if not os.path.exists(p):
        NEEDED.append((pid, w, hgt, p))
    return p


def fetch_all():
    if not NEEDED:
        return
    from concurrent.futures import ThreadPoolExecutor
    print('downloading', len(NEEDED), 'photos')
    def one(t):
        pid, w, hgt, p = t
        subprocess.run(['curl', '-sfL', '--retry', '2', '-o', p, f'https://picsum.photos/id/{pid}/{w}/{hgt}'])
    with ThreadPoolExecutor(16) as ex:
        list(ex.map(one, NEEDED))
    NEEDED.clear()


def load(path, size):
    try:
        im = Image.open(path).convert('RGB')
    except Exception:
        im = Image.new('RGB', size, INK)
    return ImageOps.fit(im, size, Image.LANCZOS)


def mix(a, b, t):
    return tuple(int(a[i] * (1 - t) + b[i] * t) for i in range(3))


def duotone(im, dark, light, amount=1.0):
    g = ImageOps.grayscale(im)
    g = ImageOps.autocontrast(g, cutoff=1)
    d = ImageOps.colorize(g, dark, light)
    return Image.blend(im, d, amount) if amount < 1 else d


def grade(im, pal, strength=0.25):
    tint = Image.new('RGB', im.size, pal)
    im = ImageEnhance.Contrast(im).enhance(1.12)
    return Image.blend(im, Image.blend(im, tint, 0.5), strength)


def vignette(im, strength=0.55):
    w, hh = im.size
    mask = Image.new('L', (w, hh), 0)
    d = ImageDraw.Draw(mask)
    d.ellipse((-w * 0.25, -hh * 0.15, w * 1.25, hh * 1.15), fill=255)
    mask = mask.filter(ImageFilter.GaussianBlur(w * 0.12))
    dark = Image.new('RGB', im.size, (0, 0, 0))
    return Image.composite(im, Image.blend(im, dark, strength), mask)


def gradient(im, top=0.0, bottom=0.85, color=INK, start=0.45):
    w, hh = im.size
    over = Image.new('RGB', im.size, color)
    mask = Image.new('L', (1, hh))
    for y in range(hh):
        t = y / hh
        a = top * max(0, (0.3 - t) / 0.3) + (bottom * max(0, (t - start) / (1 - start)) if t > start else 0)
        mask.putpixel((0, y), int(255 * min(1, a)))
    return Image.composite(over, im, mask.resize((w, hh)))


def fit_text(draw, text, fname, max_w, max_size, min_size=18):
    size = max_size
    while size > min_size:
        f = font(fname, size)
        if draw.textlength(text, font=f) <= max_w:
            return f
        size -= 2
    return font(fname, min_size)


def wrap(draw, text, fname, max_w, size, max_lines=3):
    words = text.split()
    lines = [text]
    for s in range(size, min(size, 16) - 1, -2):
        f = font(fname, s)
        lines, cur = [], ''
        for w_ in words:
            t = (cur + ' ' + w_).strip()
            if draw.textlength(t, font=f) <= max_w:
                cur = t
            else:
                if cur:
                    lines.append(cur)
                cur = w_
        lines.append(cur)
        if len(lines) <= max_lines and all(draw.textlength(l, font=f) <= max_w for l in lines):
            return f, lines
    return font(fname, max(12, min(size, 18))), lines


def draw_lines(draw, lines, f, x, y, fill, align='left', width=None, spacing=1.0, shadow=True):
    for line in lines:
        tw = draw.textlength(line, font=f)
        xx = x if align == 'left' else x + (width - tw) / 2
        if shadow:
            draw.text((xx + 2, y + 3), line, font=f, fill=(0, 0, 0))
        draw.text((xx, y), line, font=f, fill=fill)
        y += int(f.size * spacing)
    return y


GENRE_FONTS = {
    'Horror': ['Anton-Regular.ttf', 'CinzelDecorative-Bold.ttf', 'BebasNeue-Regular.ttf'],
    'Comedy': ['ArchivoBlack-Regular.ttf', 'Bungee-Regular.ttf', 'Righteous-Regular.ttf'],
    'Science Fiction': ['ChakraPetch-Bold.ttf', 'Monoton-Regular.ttf', 'RubikMonoOne-Regular.ttf'],
    'Romance': ['DMSerifDisplay-Italic.ttf', 'Italiana-Regular.ttf', 'InstrumentSerif-Italic.ttf'],
    'Western': ['Ultra-Regular.ttf', 'Limelight-Regular.ttf'],
    'Documentary': ['Manrope-Bold.ttf', 'IBMPlexMono-Medium.ttf'],
    'Animation': ['Righteous-Regular.ttf', 'Bungee-Regular.ttf'],
    'Family': ['Righteous-Regular.ttf', 'YesevaOne-Regular.ttf'],
    'Thriller': ['BebasNeue-Regular.ttf', 'Anton-Regular.ttf', 'Staatliches-Regular.ttf'],
    'Crime': ['BebasNeue-Regular.ttf', 'SpecialElite-Regular.ttf', 'Staatliches-Regular.ttf'],
    'Mystery': ['PlayfairDisplaySC-Regular.ttf', 'SpecialElite-Regular.ttf'],
    'Fantasy': ['CinzelDecorative-Bold.ttf', 'YesevaOne-Regular.ttf'],
    'History': ['PlayfairDisplaySC-Bold.ttf', 'CinzelDecorative-Bold.ttf'],
    'War': ['Anton-Regular.ttf', 'Staatliches-Regular.ttf'],
    'Music': ['Monoton-Regular.ttf', 'PoiretOne-Regular.ttf', 'Limelight-Regular.ttf'],
    'Action': ['Anton-Regular.ttf', 'BebasNeue-Regular.ttf', 'Staatliches-Regular.ttf'],
    'Adventure': ['YesevaOne-Regular.ttf', 'Anton-Regular.ttf'],
    'Drama': ['InstrumentSerif-Regular.ttf', 'DMSerifDisplay-Regular.ttf', 'PlayfairDisplaySC-Regular.ttf', 'Italiana-Regular.ttf'],
}
UPPER = {'Anton-Regular.ttf', 'BebasNeue-Regular.ttf', 'Staatliches-Regular.ttf', 'ArchivoBlack-Regular.ttf', 'Bungee-Regular.ttf', 'ChakraPetch-Bold.ttf', 'RubikMonoOne-Regular.ttf', 'Monoton-Regular.ttf', 'Ultra-Regular.ttf'}


def pick_font(genres, key):
    opts = []
    for g in genres:
        opts += GENRE_FONTS.get(g, [])
    opts = opts or GENRE_FONTS['Drama']
    return opts[h(key) % len(opts)]


def people_names(ids):
    names = {p['id']: p['name'] for p in content['people'] + content['real']['people']}
    return [names[i] for i in ids if i in names]


def credit_block(draw, W, y, text, color):
    f = font('BebasNeue-Regular.ttf', 15)
    words = text.upper()
    f, lines = wrap(draw, words, 'BebasNeue-Regular.ttf', W - 50, 15, 3)
    for line in lines:
        tw = draw.textlength(line, font=f)
        draw.text(((W - tw) / 2, y), line, font=f, fill=color)
        y += f.size + 1
    return y


def make_poster(key, title, genres, palette, photo_path, tagline='', credits='', year=None, style=None, kind='movie'):
    W, H = 400, 600
    pal = PALETTE.get(palette, PALETTE['slate'])
    base = load(photo_path, (W, H))
    fname = pick_font(genres, key)
    upper = fname in UPPER
    t = title.upper() if upper else title
    style = style if style is not None else h(key + 'style') % 5
    light = mix(pal, (255, 255, 255), 0.55)
    if 'Horror' in genres:
        base = duotone(base, (5, 5, 8), mix(pal, (220, 220, 220), 0.3), 0.85)
    if style == 0:  # full-bleed photo, title low
        im = vignette(grade(base, pal, 0.22), 0.5)
        im = gradient(im, top=0.55, bottom=0.92, start=0.42)
        d = ImageDraw.Draw(im)
        f, lines = wrap(d, t, fname, W - 48, 74 if upper else 66, 3)
        total = len(lines) * int(f.size * 0.98)
        y = H - 92 - total
        draw_lines(d, lines, f, 24, y, (255, 255, 255), 'center', W - 48, 0.98)
        if tagline:
            tf, tl = wrap(d, tagline, 'Manrope-Medium.ttf', W - 60, 15, 2)
            draw_lines(d, tl, tf, 30, 26, (235, 240, 245), 'center', W - 60, 1.3)
        credit_block(d, W, H - 64, credits, (190, 200, 210))
    elif style == 1:  # photo top, colour band
        im = Image.new('RGB', (W, H), mix(pal, INK, 0.72))
        top = vignette(grade(load(photo_path, (W, 400)), pal, 0.3), 0.35)
        im.paste(top, (0, 0))
        im = gradient(im, bottom=0.0)
        band = Image.new('RGB', (W, 210), mix(pal, INK, 0.72))
        im.paste(band, (0, 400))
        d = ImageDraw.Draw(im)
        d.rectangle((0, 396, W, 400), fill=light)
        f, lines = wrap(d, t, fname, W - 50, 60 if len(t) < 12 else 46, 2)
        draw_lines(d, lines, f, 25, 418 if len(lines) == 2 else 440, (255, 255, 255), 'center', W - 50, 1.0, shadow=False)
        credit_block(d, W, H - 60, credits, mix(light, (255, 255, 255), 0.4))
    elif style == 2:  # framed
        bg = mix(pal, INK, 0.55)
        im = Image.new('RGB', (W, H), bg)
        inner = grade(load(photo_path, (W - 60, 340)), pal, 0.25)
        im.paste(inner, (30, 150))
        d = ImageDraw.Draw(im)
        d.rectangle((30, 150, W - 31, 489), outline=light, width=2)
        f, lines = wrap(d, t, fname, W - 60, 58, 2)
        y = 138 - len(lines) * int(f.size * 1.0)
        draw_lines(d, lines, f, 30, max(30, y), (255, 255, 255), 'center', W - 60, 1.0, shadow=False)
        if tagline:
            tf, tl = wrap(d, tagline, 'InstrumentSerif-Italic.ttf', W - 60, 22, 2)
            draw_lines(d, tl, tf, 30, 505, light, 'center', W - 60, 1.1, shadow=False)
        credit_block(d, W, H - 42, credits, mix(light, bg, 0.2))
    elif style == 3:  # giant type over faded photo
        im = duotone(base, mix(pal, INK, 0.85), light, 1.0)
        im = ImageEnhance.Brightness(im).enhance(0.7)
        d = ImageDraw.Draw(im)
        f, lines = wrap(d, t, fname, W - 36, 120, 4)
        total = len(lines) * int(f.size * 0.95)
        y = (H - total) / 2 - 20
        draw_lines(d, lines, f, 18, y, (255, 255, 255), 'center', W - 36, 0.95)
        if year:
            yf = font('Manrope-Bold.ttf', 14)
            d.text((W / 2 - d.textlength(str(year), font=yf) / 2, H - 38), str(year), font=yf, fill=(230, 235, 240))
        if credits:
            credit_block(d, W, 28, credits.split('·')[0], (230, 235, 240))
    else:  # circle
        bg = mix(pal, INK, 0.35)
        im = Image.new('RGB', (W, H), bg)
        circ = grade(load(photo_path, (300, 300)), pal, 0.2)
        mask = Image.new('L', (300, 300), 0)
        ImageDraw.Draw(mask).ellipse((0, 0, 299, 299), fill=255)
        im.paste(circ, (50, 70), mask)
        d = ImageDraw.Draw(im)
        d.ellipse((46, 66, 354, 374), outline=mix(light, (255, 255, 255), 0.5), width=3)
        f, lines = wrap(d, t, fname, W - 50, 62, 2)
        draw_lines(d, lines, f, 25, 400, (255, 255, 255), 'center', W - 50, 1.0, shadow=False)
        credit_block(d, W, H - 58, credits, mix(light, (255, 255, 255), 0.5))
    return im


def save(im, rel, quality=82):
    path = os.path.join(OUT, rel)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    if path.endswith('.png'):
        im.save(path, optimize=True)
    else:
        im.convert('RGB').save(path, quality=quality, optimize=True, progressive=True)
    return path


def exists(rel):
    return not FORCE and os.path.exists(os.path.join(OUT, rel))


def dominant(path):
    im = Image.open(path).convert('RGB').resize((48, 48))
    best, score = (64, 159, 212), -1
    buckets = {}
    for px in im.getdata():
        r, g, b = px
        hh, l, s = colorsys.rgb_to_hls(r / 255, g / 255, b / 255)
        if l < 0.12 or l > 0.9 or s < 0.18:
            continue
        k = (round(hh * 12), round(l * 4))
        c = buckets.setdefault(k, [0, 0, 0, 0])
        c[0] += r; c[1] += g; c[2] += b; c[3] += 1
    for c in buckets.values():
        if c[3] > score:
            score = c[3]
            best = (c[0] // c[3], c[1] // c[3], c[2] // c[3])
    hh, l, s = colorsys.rgb_to_hls(*(v / 255 for v in best))
    r, g, b = colorsys.hls_to_rgb(hh, max(0.42, min(0.62, l)), max(0.45, s))
    return '#%02x%02x%02x' % (int(r * 255), int(g * 255), int(b * 255))


# --------------------------------------------------------------------------- jobs

def real_movies(jobs):
    for m in content['real']['movies']:
        src = os.path.join(CACHE, 'real', m['id'])
        stills = [os.path.join(src, f'still-{i}.jpg') for i in range(10) if os.path.exists(os.path.join(src, f'still-{i}.jpg'))]
        poster = os.path.join(src, 'poster.jpg')

        def job(m=m, stills=stills, poster=poster):
            if not exists(f'poster/{m["id"]}.jpg'):
                if os.path.exists(poster):
                    p = Image.open(poster).convert('RGB')
                    bg = ImageOps.fit(p, (400, 600)).filter(ImageFilter.GaussianBlur(18))
                    fg = ImageOps.contain(p, (400, 600), Image.LANCZOS)
                    bg.paste(fg, ((400 - fg.width) // 2, (600 - fg.height) // 2))
                    save(bg, f'poster/{m["id"]}.jpg')
                elif stills:
                    cred = ' · '.join(people_names([c['person'] for c in m.get('cast', [])][:3]))
                    save(make_poster(m['id'], m['title'], m['genres'], m.get('palette', 'slate'), stills[3 % len(stills)], m.get('tagline', ''), cred, m['year'], style=0), f'poster/{m["id"]}.jpg')
            if stills and not exists(f'backdrop/{m["id"]}.jpg'):
                pick = stills[min(len(stills) - 1, 3)]
                save(ImageOps.fit(Image.open(pick).convert('RGB'), (1280, 720), Image.LANCZOS), f'backdrop/{m["id"]}.jpg', 80)
            for i, st in enumerate(stills):
                if not exists(f'still/{m["id"]}-{i}.jpg'):
                    save(ImageOps.fit(Image.open(st).convert('RGB'), (480, 270), Image.LANCZOS), f'still/{m["id"]}-{i}.jpg', 76)
        jobs.append(job)
    for p in content['real']['people']:
        src = os.path.join(CACHE, 'real', 'people', p['id'] + '.jpg')
        if os.path.exists(src):
            jobs.append(lambda p=p, src=src: exists(f'person/{p["id"]}.jpg') or save(headshot(Image.open(src)), f'person/{p["id"]}.jpg', 80))


def headshot(im):
    im = im.convert('RGB')
    w, hh = im.size
    side = min(w, hh)
    top = int(max(0, (hh - side) * 0.18))
    im = im.crop(((w - side) // 2, top, (w - side) // 2 + side, top + side)).resize((300, 300), Image.LANCZOS)
    g = ImageOps.autocontrast(ImageOps.grayscale(im), cutoff=1)
    return ImageOps.colorize(g, (10, 20, 30), (240, 244, 248), mid=(120, 140, 155))


def invented_movies(jobs, alloc_p, alloc_b):
    for m in content['movies']:
        pp = need(alloc_p.take(), 400, 600)
        bp = need(alloc_b.take(), 1280, 720)
        cast = people_names([c['person'] for c in m.get('cast', [])][:3])
        director = people_names([m['director']]) if m.get('director') else []
        cred = ('A film by ' + director[0] + ' · ' if director else '') + ' · '.join(cast)

        def job(m=m, pp=pp, bp=bp, cred=cred):
            if not exists(f'poster/{m["id"]}.jpg'):
                save(make_poster(m['id'], m['title'], m['genres'], m.get('palette', 'slate'), pp, m.get('tagline', ''), cred, m['year']), f'poster/{m["id"]}.jpg')
            if not exists(f'backdrop/{m["id"]}.jpg'):
                save(grade(load(bp, (1280, 720)), PALETTE.get(m.get('palette'), PALETTE['slate']), 0.14), f'backdrop/{m["id"]}.jpg', 78)
        jobs.append(job)


def shows(jobs, alloc_p, alloc_b, alloc_s):
    for s in content['shows']:
        pp = need(alloc_p.take(), 400, 600)
        bp = need(alloc_b.take(), 1280, 720)
        eps = []
        for season in s['seasons']:
            for i, _ in enumerate(season['episodes']):
                eps.append((f'{s["id"]}-s{season["number"]}e{i + 1}', need(alloc_s.take(), 480, 270)))
        cast = people_names([c['person'] for c in s.get('cast', [])][:3])
        pal = PALETTE.get(s.get('palette'), PALETTE['slate'])

        def job(s=s, pp=pp, bp=bp, eps=eps, cast=cast, pal=pal):
            genres = s['genres'] + (['Animation'] if s['kind'] == 'anime' else [])
            if not exists(f'poster/{s["id"]}.jpg'):
                cred = (s.get('network', '') + ' · ' if s.get('network') else '') + ' · '.join(cast)
                save(make_poster(s['id'], s['title'], genres, s.get('palette', 'slate'), pp, '', cred, s.get('yearStart'), kind='show'), f'poster/{s["id"]}.jpg')
            if not exists(f'backdrop/{s["id"]}.jpg'):
                save(grade(load(bp, (1280, 720)), pal, 0.14), f'backdrop/{s["id"]}.jpg', 78)
            for eid, path in eps:
                if not exists(f'still/{eid}.jpg'):
                    save(grade(load(path, (480, 270)), pal, 0.12), f'still/{eid}.jpg', 74)
        jobs.append(job)


def music(jobs, alloc_sq, alloc_people):
    for a in content['music']:
        ap = need(alloc_people.take(), 500, 500)
        pal = PALETTE.get(a.get('palette'), PALETTE['slate'])
        albums = [(al, need(alloc_sq.take(), 500, 500)) for al in a['albums']]

        def job(a=a, ap=ap, albums=albums, pal=pal):
            if not exists(f'artist/{a["id"]}.jpg'):
                im = grade(load(ap, (500, 500)), pal, 0.2)
                save(vignette(im, 0.35), f'artist/{a["id"]}.jpg', 80)
            for al, path in albums:
                if not exists(f'album/{al["id"]}.jpg'):
                    save(album_cover(al, a, path, pal), f'album/{al["id"]}.jpg', 80)
        jobs.append(job)


def album_cover(al, a, path, pal):
    S = 500
    style = h(al['id']) % 4
    fname = ['Italiana-Regular.ttf', 'BebasNeue-Regular.ttf', 'PoiretOne-Regular.ttf', 'DMSerifDisplay-Regular.ttf', 'Monoton-Regular.ttf', 'Righteous-Regular.ttf'][h(al['id'] + 'f') % 6]
    title = al['title'].upper() if fname in UPPER else al['title']
    if style == 0:
        im = grade(load(path, (S, S)), pal, 0.25)
        d = ImageDraw.Draw(im)
        f = fit_text(d, a['name'].upper(), 'Manrope-Bold.ttf', S - 60, 26)
        d.text((30, 28), a['name'].upper(), font=f, fill=(255, 255, 255))
        tf, lines = wrap(d, title, fname, S - 60, 52, 2)
        draw_lines(d, lines, tf, 30, S - 40 - len(lines) * tf.size, (255, 255, 255))
    elif style == 1:
        im = duotone(load(path, (S, S)), mix(pal, INK, 0.8), mix(pal, (255, 255, 255), 0.5))
        d = ImageDraw.Draw(im)
        tf, lines = wrap(d, title, fname, S - 80, 70, 3)
        draw_lines(d, lines, tf, 40, S / 2 - len(lines) * tf.size / 2, (255, 255, 255), 'center', S - 80)
        f = font('Manrope-Medium.ttf', 18)
        d.text((S / 2 - d.textlength(a['name'], font=f) / 2, S - 50), a['name'], font=f, fill=(240, 240, 240))
    elif style == 2:
        im = Image.new('RGB', (S, S), mix(pal, (240, 236, 228), 0.85))
        inner = grade(load(path, (360, 360)), pal, 0.15)
        im.paste(inner, (70, 50))
        d = ImageDraw.Draw(im)
        f = fit_text(d, title, fname, S - 80, 34)
        d.text((S / 2 - d.textlength(title, font=f) / 2, 425), title, font=f, fill=mix(pal, INK, 0.7))
        f2 = font('Manrope-Bold.ttf', 14)
        d.text((S / 2 - d.textlength(a['name'].upper(), font=f2) / 2, 466), a['name'].upper(), font=f2, fill=mix(pal, INK, 0.5))
    else:
        im = Image.new('RGB', (S, S), mix(pal, INK, 0.3))
        d = ImageDraw.Draw(im)
        rnd = random.Random(al['id'])
        for i in range(7):
            c = mix(pal, (255, 255, 255) if i % 2 else INK, rnd.random() * 0.6)
            r = rnd.randint(60, 260)
            x, y = rnd.randint(-40, S), rnd.randint(-40, S)
            d.ellipse((x - r, y - r, x + r, y + r), fill=c)
        im = im.filter(ImageFilter.GaussianBlur(28))
        d = ImageDraw.Draw(im)
        tf, lines = wrap(d, title, fname, S - 70, 64, 3)
        draw_lines(d, lines, tf, 35, 40, (255, 255, 255))
        f = font('Manrope-Bold.ttf', 18)
        d.text((35, S - 56), a['name'], font=f, fill=(255, 255, 255))
    return im


def books(jobs, alloc):
    for b in content['audiobooks']:
        pp = need(alloc.take(), 400, 600)

        def job(b=b, pp=pp):
            if exists(f'book/{b["id"]}.jpg'):
                return
            W, H = 400, 600
            pal = PALETTE.get(b.get('palette'), PALETTE['slate'])
            style = h(b['id']) % 3
            if b.get('publicDomain') or style == 0:  # classic cloth-bound look
                bg = mix(pal, INK, 0.45)
                im = Image.new('RGB', (W, H), bg)
                d = ImageDraw.Draw(im)
                gold = (214, 186, 120)
                d.rectangle((22, 22, W - 23, H - 23), outline=gold, width=2)
                d.rectangle((30, 30, W - 31, H - 31), outline=gold, width=1)
                tf, lines = wrap(d, b['title'], 'PlayfairDisplaySC-Bold.ttf', W - 90, 50, 4)
                y = draw_lines(d, lines, tf, 45, 150, gold, 'center', W - 90, 1.08, shadow=False)
                d.line((W / 2 - 40, y + 20, W / 2 + 40, y + 20), fill=gold, width=2)
                af = font('Italiana-Regular.ttf', 28)
                d.text((W / 2 - d.textlength(b['author'], font=af) / 2, y + 44), b['author'], font=af, fill=gold)
                nf = font('Manrope-Medium.ttf', 13)
                n = 'Read by ' + b.get('narrator', '')
                d.text((W / 2 - d.textlength(n, font=nf) / 2, H - 70), n, font=nf, fill=mix(gold, bg, 0.3))
            else:
                im = Image.new('RGB', (W, H), (240, 234, 222))
                photo = duotone(load(pp, (W, 330)), mix(pal, INK, 0.7), mix(pal, (255, 255, 255), 0.6))
                im.paste(photo, (0, 150 if style == 1 else 270))
                d = ImageDraw.Draw(im)
                ty = 24 if style == 1 else 30
                af = font('Manrope-Bold.ttf', 16)
                d.text((30, ty), b['author'].upper(), font=af, fill=mix(pal, INK, 0.6))
                tf, lines = wrap(d, b['title'], 'DMSerifDisplay-Regular.ttf', W - 60, 44, 3)
                draw_lines(d, lines, tf, 30, ty + 30 if style == 2 else 494, mix(pal, INK, 0.75), shadow=False)
                if b.get('series'):
                    sf = font('Manrope-Bold.ttf', 12)
                    s = f"{b['series']['name']} · Book {b['series']['number']}".upper()
                    d.text((30, H - 30), s, font=sf, fill=mix(pal, INK, 0.5))
            save(im, f'book/{b["id"]}.jpg')
        jobs.append(job)


def channels(jobs):
    for c in content['live']['channels']:
        def job(c=c):
            if exists(f'logo/{c["id"]}.png'):
                return
            W, H = 400, 225
            pal = PALETTE.get(c.get('palette'), PALETTE['slate'])
            im = Image.new('RGBA', (W, H), mix(pal, INK, 0.25) + (255,))
            d = ImageDraw.Draw(im)
            style = h(c['id']) % 4
            fname = ['Anton-Regular.ttf', 'Righteous-Regular.ttf', 'ArchivoBlack-Regular.ttf', 'DMSerifDisplay-Regular.ttf', 'ChakraPetch-Bold.ttf', 'Staatliches-Regular.ttf'][h(c['id'] + 'f') % 6]
            name = c['name'].upper() if fname in UPPER else c['name']
            if style == 0:
                f, lines = wrap(d, name, fname, W - 60, 64, 2)
                draw_lines(d, lines, f, 30, H / 2 - len(lines) * f.size / 2, (255, 255, 255), 'center', W - 60, 1.0, shadow=False)
            elif style == 1:
                d.ellipse((30, 52, 150, 172), fill=(255, 255, 255, 255))
                nf = fit_text(d, c['number'], 'Anton-Regular.ttf', 90, 60)
                d.text((90 - d.textlength(c['number'], font=nf) / 2, 112 - nf.size * 0.62), c['number'], font=nf, fill=mix(pal, INK, 0.35) + (255,))
                f, lines = wrap(d, name, fname, 210, 44, 3)
                draw_lines(d, lines, f, 170, H / 2 - len(lines) * f.size / 2, (255, 255, 255), shadow=False)
            elif style == 2:
                cs = c.get('callsign', name[:4]).upper()
                f = fit_text(d, cs, 'Anton-Regular.ttf', W - 80, 110)
                d.text((W / 2 - d.textlength(cs, font=f) / 2, 30), cs, font=f, fill=(255, 255, 255))
                sf = fit_text(d, c['name'], 'Manrope-Bold.ttf', W - 80, 20)
                d.text((W / 2 - d.textlength(c['name'], font=sf) / 2, H - 50), c['name'], font=sf, fill=mix(pal, (255, 255, 255), 0.6))
            else:
                d.rectangle((0, H - 44, W, H), fill=(255, 255, 255, 255))
                f, lines = wrap(d, name, fname, W - 60, 60, 2)
                draw_lines(d, lines, f, 30, (H - 44) / 2 - len(lines) * f.size / 2, (255, 255, 255), 'center', W - 60, 1.0, shadow=False)
                sf = font('Manrope-Bold.ttf', 20)
                label = c.get('group', '').upper()
                d.text((W / 2 - d.textlength(label, font=sf) / 2, H - 36), label, font=sf, fill=mix(pal, INK, 0.3))
            save(im, f'logo/{c["id"]}.png')
        jobs.append(job)


def personal(jobs, alloc_photo, alloc_video):
    for pa in content['personal']['photoAlbums']:
        photos = [(f'{pa["id"]}-{n + 1}', need(alloc_photo.take(), 1200, 800)) for n in range(pa['count'])]

        def job(photos=photos):
            for pid, path in photos:
                if not exists(f'photo/{pid}.jpg'):
                    im = load(path, (1200, 800))
                    save(im, f'photo/{pid}.jpg', 78)
                if not exists(f'photothumb/{pid}.jpg'):
                    save(ImageOps.fit(load(path, (1200, 800)), (360, 360)), f'photothumb/{pid}.jpg', 74)
        jobs.append(job)
    for hv in content['personal']['homeVideos']:
        path = need(alloc_video.take(), 640, 360)
        jobs.append(lambda hv=hv, path=path: exists(f'homevideo/{hv["id"]}.jpg') or save(ImageEnhance.Color(load(path, (640, 360))).enhance(1.1), f'homevideo/{hv["id"]}.jpg', 76))


def invented_people(jobs):
    folder = os.path.join(CACHE, 'portraits')
    portraits = sorted(f for f in os.listdir(folder) if f.endswith('.jpg')) if os.path.isdir(folder) else []
    if not portraits:
        return
    for i, p in enumerate(content['people']):
        src = os.path.join(folder, portraits[i % len(portraits)])
        jobs.append(lambda p=p, src=src: exists(f'person/{p["id"]}.jpg') or save(headshot(Image.open(src)), f'person/{p["id"]}.jpg', 80))


def write_manifest():
    lines = ['/* Generated by scripts/art/build_art.py. Do not edit. */', "import type {ImageSourcePropType} from 'react-native';", '', 'export const images: Record<string, ImageSourcePropType> = {']
    tints = {}
    tint_cache_path = os.path.join(CACHE, 'tints.json')
    tint_cache = json.load(open(tint_cache_path)) if os.path.exists(tint_cache_path) else {}
    for cat in sorted(os.listdir(OUT)):
        d = os.path.join(OUT, cat)
        if not os.path.isdir(d):
            continue
        for f in sorted(os.listdir(d)):
            key = f'{cat}/{os.path.splitext(f)[0]}'
            lines.append(f"  '{key}': require('../../../assets/art/{cat}/{f}'),")
            if cat in ('poster', 'album', 'artist', 'book', 'logo', 'photo', 'homevideo', 'backdrop', 'person'):
                full = os.path.join(d, f)
                stamp = f'{os.path.getmtime(full):.0f}'
                if tint_cache.get(key, [None])[0] != stamp:
                    tint_cache[key] = [stamp, dominant(full)]
                tints[key] = tint_cache[key][1]
    lines.append('};')
    lines.append('')
    lines.append('export const tints: Record<string, string> = ' + json.dumps(tints, indent=0, sort_keys=True).replace('\n', ' ') + ';')
    json.dump(tint_cache, open(tint_cache_path, 'w'))
    with open(os.path.join(ROOT, 'src/data/fixtures/art.generated.ts'), 'w') as fh:
        fh.write('\n'.join(lines) + '\n')
    total = sum(os.path.getsize(os.path.join(dp, f)) for dp, _, fs in os.walk(OUT) for f in fs)
    print('manifest written;', len(tints), 'tints; assets', round(total / 1e6, 1), 'MB')


if __name__ == '__main__':
    jobs = []
    people_pool = PEOPLE_PHOTOS + [158, 451, 452, 1069, 669, 773, 836]
    alloc_poster = Allocator(POOL, 'poster')
    alloc_back = Allocator(POOL, 'backdrop')
    alloc_still = Allocator(POOL, 'still')
    alloc_sq = Allocator(POOL, 'square')
    alloc_people = Allocator(people_pool + POOL[:40], 'people')
    alloc_book = Allocator(POOL, 'book')
    alloc_photo = Allocator([i for i in POOL], 'photo')
    alloc_video = Allocator(PEOPLE_PHOTOS + POOL, 'video')
    if not ONLY or 'real' in ONLY:
        real_movies(jobs)
    if not ONLY or 'movies' in ONLY:
        invented_movies(jobs, alloc_poster, alloc_back)
    if not ONLY or 'shows' in ONLY:
        shows(jobs, alloc_poster, alloc_back, alloc_still)
    if not ONLY or 'music' in ONLY:
        music(jobs, alloc_sq, alloc_people)
    if not ONLY or 'books' in ONLY:
        books(jobs, alloc_book)
    if not ONLY or 'live' in ONLY:
        channels(jobs)
    if not ONLY or 'personal' in ONLY:
        personal(jobs, alloc_photo, alloc_video)
    if not ONLY or 'people' in ONLY:
        invented_people(jobs)
    fetch_all()
    for j in jobs:
        try:
            j()
        except Exception as e:  # keep going; a missing image falls back to the designed placeholder
            print('art job failed:', e)
    write_manifest()
