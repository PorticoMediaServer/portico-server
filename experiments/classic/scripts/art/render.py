#!/usr/bin/env python3
"""
Composes bundled artwork from .art-cache/src into assets/art and writes the
require() manifest src/data/fixtures/art.generated.ts.

Posters and covers for invented titles are generated here: a photograph,
graded to a palette, with a typographic treatment. Real titles use their
Commons posters and stills. Run after fetch.py.
"""
import hashlib, json, math, os, random
from concurrent.futures import ProcessPoolExecutor
from PIL import Image, ImageDraw, ImageFilter, ImageFont, ImageOps, ImageEnhance, ImageChops

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
CACHE = os.path.join(ROOT, '.art-cache')
SRC = os.path.join(CACHE, 'src')
OUT = os.path.join(ROOT, 'assets', 'art')
plan = json.load(open(os.path.join(CACHE, 'plan.json')))

F = '/System/Library/Fonts/'
S = '/System/Library/Fonts/Supplemental/'
FONTS = {
    'din': (S + 'DIN Condensed Bold.ttf', 0), 'impact': (S + 'Impact.ttf', 0), 'futura': (S + 'Futura.ttc', 0), 'futuraBold': (S + 'Futura.ttc', 2),
    'futuraCond': (S + 'Futura.ttc', 4), 'didot': (S + 'Didot.ttc', 0), 'didotBold': (S + 'Didot.ttc', 2), 'bodoni': (S + 'Bodoni 72.ttc', 0),
    'baskerville': (S + 'Baskerville.ttc', 0), 'baskervilleItalic': (S + 'Baskerville.ttc', 1), 'gill': (S + 'GillSans.ttc', 0), 'gillLight': (S + 'GillSans.ttc', 2),
    'avenir': (F + 'Avenir Next.ttc', 0), 'avenirBold': (F + 'Avenir Next.ttc', 2), 'avenirHeavy': (F + 'Avenir Next.ttc', 8), 'avenirCond': (F + 'Avenir Next Condensed.ttc', 2),
    'copperplate': (S + 'Copperplate.ttc', 0), 'rockwell': (S + 'Rockwell.ttc', 2), 'optima': (F + 'Optima.ttc', 0), 'georgia': (S + 'Georgia Bold.ttf', 0),
    'helvBold': (F + 'HelveticaNeue.ttc', 1), 'helvLight': (F + 'HelveticaNeue.ttc', 7), 'hoefler': (S + 'Hoefler Text.ttc', 0), 'palatino': (F + 'Palatino.ttc', 0),
}
_font_cache = {}
def font(name, size):
    key = (name, size)
    if key not in _font_cache:
        path, idx = FONTS[name]
        try: _font_cache[key] = ImageFont.truetype(path, size, index=idx)
        except Exception: _font_cache[key] = ImageFont.truetype(FONTS['avenir'][0], size, index=0)
    return _font_cache[key]

PALETTES = [
    ((12, 22, 38), (64, 159, 212), (237, 245, 250)),   # Portico night
    ((28, 14, 10), (214, 120, 60), (250, 232, 210)),   # ember
    ((10, 26, 24), (76, 176, 150), (225, 245, 236)),   # sea glass
    ((30, 12, 30), (200, 92, 150), (250, 226, 240)),   # neon rose
    ((20, 20, 24), (230, 190, 100), (250, 244, 225)),  # gold
    ((8, 18, 36), (120, 140, 230), (228, 232, 255)),   # indigo
    ((34, 10, 12), (220, 70, 70), (255, 230, 225)),    # crimson
    ((14, 24, 14), (150, 190, 90), (238, 248, 222)),   # moss
    ((24, 22, 30), (170, 150, 200), (240, 236, 250)),  # lilac
    ((10, 20, 26), (90, 190, 210), (225, 248, 252)),   # cyan
]

def h(s):
    return int(hashlib.md5(s.encode()).hexdigest(), 16)

def palette(key):
    return PALETTES[h(key) % len(PALETTES)]

def load(name):
    p = os.path.join(SRC, name)
    return Image.open(p).convert('RGB') if os.path.exists(p) else None

def cover(img, w, hgt):
    return ImageOps.fit(img, (w, hgt), Image.LANCZOS, centering=(0.5, 0.45))

def grade(img, pal, amount=0.45, contrast=1.1, sat=0.85):
    img = ImageEnhance.Color(img).enhance(sat)
    img = ImageEnhance.Contrast(img).enhance(contrast)
    g = ImageOps.grayscale(img)
    tinted = ImageOps.colorize(g, pal[0], pal[2], mid=pal[1])
    return Image.blend(img, tinted, amount)

def duotone(img, pal):
    g = ImageOps.autocontrast(ImageOps.grayscale(img), cutoff=2)
    return ImageOps.colorize(g, pal[0], pal[2], mid=pal[1])

def mono(img, warm=False):
    g = ImageOps.autocontrast(ImageOps.grayscale(img), cutoff=1)
    if warm:
        return ImageOps.colorize(g, (22, 16, 10), (236, 222, 196), mid=(140, 112, 80))
    return ImageOps.colorize(g, (8, 10, 12), (238, 240, 242), mid=(120, 124, 128))

def vgradient(w, hgt, top, bottom, start=0.0, end=1.0):
    """Vertical alpha gradient from `top` alpha to `bottom` alpha between start..end."""
    mask = Image.new('L', (1, hgt))
    for y in range(hgt):
        t = y / max(1, hgt - 1)
        if t <= start: a = top
        elif t >= end: a = bottom
        else: a = top + (bottom - top) * ((t - start) / (end - start))
        mask.putpixel((0, y), int(a))
    return mask.resize((w, hgt))

def darken_bottom(img, color=(0, 0, 0), start=0.45, strength=235):
    w, hh = img.size
    overlay = Image.new('RGB', (w, hh), color)
    return Image.composite(overlay, img, vgradient(w, hh, 0, strength, start, 1.0))

def darken_top(img, color=(0, 0, 0), end=0.45, strength=200):
    w, hh = img.size
    overlay = Image.new('RGB', (w, hh), color)
    return Image.composite(overlay, img, vgradient(w, hh, strength, 0, 0, end))

def vignette(img, strength=0.55):
    w, hh = img.size
    mask = Image.new('L', (w, hh), 0)
    d = ImageDraw.Draw(mask)
    d.ellipse((-w * 0.25, -hh * 0.2, w * 1.25, hh * 1.2), fill=255)
    mask = mask.filter(ImageFilter.GaussianBlur(w * 0.18))
    dark = ImageEnhance.Brightness(img).enhance(1 - strength)
    return Image.composite(img, dark, mask)

def grain(img, amount=18, seed=0):
    rnd = random.Random(seed)
    w, hh = img.size
    noise = Image.effect_noise((w, hh), amount).convert('RGB')
    return ImageChops.add(ImageChops.subtract(img, Image.new('RGB', (w, hh), (amount // 2,) * 3)), ImageChops.multiply(noise, Image.new('RGB', (w, hh), (40, 40, 40))), 1, 0)

def wrap(draw, text, fnt, max_w):
    words, lines, cur = text.split(), [], ''
    for wd in words:
        t = (cur + ' ' + wd).strip()
        if draw.textlength(t, font=fnt) <= max_w or not cur:
            cur = t
        else:
            lines.append(cur); cur = wd
    if cur: lines.append(cur)
    return lines

def fit_text(draw, text, name, max_w, max_size, min_size=18, max_lines=3, upper=False):
    txt = text.upper() if upper else text
    size = max_size
    while size > min_size:
        f = font(name, size)
        lines = wrap(draw, txt, f, max_w)
        if len(lines) <= max_lines and all(draw.textlength(l, font=f) <= max_w for l in lines):
            return f, lines
        size -= 3
    f = font(name, min_size)
    return f, wrap(draw, txt, f, max_w)[:max_lines]

def draw_lines(draw, lines, fnt, x, y, fill, align='left', width=None, spacing=1.0, tracking=0):
    asc, desc = fnt.getmetrics()
    lh = int((asc + desc) * spacing)
    for i, l in enumerate(lines):
        tw = draw.textlength(l, font=fnt) + tracking * max(0, len(l) - 1)
        xx = x if align == 'left' else (x + (width - tw) / 2 if align == 'center' else x + width - tw)
        if tracking:
            cx = xx
            for ch in l:
                draw.text((cx, y + i * lh), ch, font=fnt, fill=fill)
                cx += draw.textlength(ch, font=fnt) + tracking
        else:
            draw.text((xx, y + i * lh), l, font=fnt, fill=fill)
    return y + len(lines) * lh

def credit_block(draw, W, y, fill, names):
    f = font('avenirCond', 17)
    line = '   '.join(n.upper() for n in names)
    lines = wrap(draw, line, f, W - 90)
    draw_lines(draw, lines[:2], f, 45, y, fill, 'center', W - 90)

def people_names(ids):
    idx = {p['id']: p['name'] for p in plan['people']}
    return [idx.get(i, '') for i in ids]

# ── Posters ─────────────────────────────────────────────────────────────
def poster_generated(item, photo, kind='movie'):
    W, H = 720, 1080
    key = item['id']
    pal = palette(key)
    style = item['art'].get('style', h(key) % 8)
    title = item['title']
    img = cover(photo, W, H)
    rnd = random.Random(h(key))
    cast = [c['person'] for c in item.get('cast', [])][:3]
    names = people_names(cast)
    year = str(item.get('year', ''))
    if kind == 'anime':
        style = [3, 0, 6, 2][h(key) % 4]
    if style == 0:  # bottom block, condensed title
        img = grade(img, pal, 0.35)
        img = darken_bottom(img, (pal[0][0] // 2, pal[0][1] // 2, pal[0][2] // 2), 0.42, 250)
        d = ImageDraw.Draw(img)
        f, lines = fit_text(d, title, 'din' if rnd.random() < .6 else 'impact', W - 100, 170, 60, 3, upper=True)
        asc, desc = f.getmetrics()
        y = H - 150 - len(lines) * (asc + desc)
        if item.get('tagline'):
            ft = font('avenir', 28)
            d.text((50, y - 50), item['tagline'], font=ft, fill=pal[2])
        draw_lines(d, lines, f, 50, y, (255, 255, 255))
        credit_block(d, W, H - 92, (220, 225, 230), names + [year])
    elif style == 1:  # serif, centred, rules
        img = vignette(ImageEnhance.Brightness(grade(img, pal, 0.5, 1.05, 0.7)).enhance(0.72), 0.6)
        img = darken_bottom(img, (6, 8, 12), 0.4, 215)
        d = ImageDraw.Draw(img)
        f, lines = fit_text(d, title, rnd.choice(['didot', 'bodoni', 'baskerville']), W - 120, 120, 48, 3)
        asc, desc = f.getmetrics()
        block = len(lines) * (asc + desc)
        y = int(H * 0.66 - block / 2)
        d.line((W / 2 - 60, y - 34, W / 2 + 60, y - 34), fill=pal[2], width=2)
        yy = draw_lines(d, lines, f, 60, y, (250, 248, 244), 'center', W - 120)
        d.line((W / 2 - 60, yy + 22, W / 2 + 60, yy + 22), fill=pal[2], width=2)
        fs = font('gill', 26)
        n = ('  ·  '.join(names[:2])).upper()
        d.text(((W - d.textlength(n, font=fs)) / 2, 70), n, font=fs, fill=(235, 235, 235))
        credit_block(d, W, H - 86, (205, 205, 210), [year])
    elif style == 2:  # title on top, spaced sans
        img = grade(img, pal, 0.3, 1.15, 0.9)
        img = darken_top(img, (0, 0, 0), 0.42, 215)
        img = darken_bottom(img, (0, 0, 0), 0.8, 190)
        d = ImageDraw.Draw(img)
        f, lines = fit_text(d, title, 'futuraBold', W - 110, 96, 40, 3, upper=True)
        draw_lines(d, lines, f, 55, 80, (255, 255, 255), 'center', W - 110, 1.0, 4)
        fs = font('futura', 24)
        n = names[0].upper() if names else ''
        d.text(((W - d.textlength(n, font=fs)) / 2, 50 - 10), n, font=fs, fill=pal[2])
        credit_block(d, W, H - 80, (220, 220, 220), names[1:] + [year])
    elif style == 3:  # duotone, huge title
        img = duotone(img, pal)
        img = darken_bottom(img, pal[0], 0.55, 200)
        d = ImageDraw.Draw(img)
        f, lines = fit_text(d, title, rnd.choice(['avenirHeavy', 'helvBold', 'impact']), W - 90, 150, 56, 3, upper=kind != 'anime' or True)
        asc, desc = f.getmetrics()
        y = H - 130 - len(lines) * int((asc + desc) * 0.92)
        draw_lines(d, lines, f, 45, y, (255, 255, 255), 'left', W - 90, 0.92)
        credit_block(d, W, H - 80, pal[2], names[:2] + [year])
    elif style == 4:  # framed photo on a colour field
        bg = Image.new('RGB', (W, H), tuple(int(c * 0.9) for c in pal[0]))
        inner = cover(grade(photo, pal, 0.25, 1.1, 0.9), W - 120, int(H * 0.62))
        bg.paste(inner, (60, 60))
        img = bg
        d = ImageDraw.Draw(img)
        f, lines = fit_text(d, title, rnd.choice(['baskervilleItalic', 'hoefler', 'palatino']), W - 120, 92, 40, 3)
        y = int(H * 0.62) + 100
        yy = draw_lines(d, lines, f, 60, y, pal[2], 'left', W - 120)
        fs = font('gill', 26)
        d.text((60, yy + 20), ('  '.join(names[:2])).upper(), font=fs, fill=(210, 214, 220))
        d.text((60, H - 70), year, font=font('gill', 24), fill=pal[1])
    elif style == 5:  # minimal: small title, lots of image
        img = grade(img, pal, 0.25, 1.05, 0.8)
        img = darken_bottom(img, (0, 0, 0), 0.72, 200)
        d = ImageDraw.Draw(img)
        f, lines = fit_text(d, title, 'optima', W - 140, 72, 36, 2, upper=True)
        asc, desc = f.getmetrics()
        y = H - 110 - len(lines) * (asc + desc)
        draw_lines(d, lines, f, 70, y, (255, 255, 255), 'center', W - 140, 1.0, 6)
        fs = font('avenir', 22)
        t = (item.get('tagline') or '').upper()
        if t: d.text(((W - d.textlength(t, font=fs)) / 2, 60), t, font=fs, fill=(235, 235, 235))
        credit_block(d, W, H - 70, (200, 200, 205), [year])
    elif style == 6:  # colour band
        band = int(H * 0.34)
        top = cover(grade(photo, pal, 0.35), W, H - band)
        img = Image.new('RGB', (W, H), pal[1])
        img.paste(top, (0, 0))
        d = ImageDraw.Draw(img)
        d.rectangle((0, H - band, W, H), fill=tuple(int(c * 0.55) for c in pal[1]))
        f, lines = fit_text(d, title, rnd.choice(['futuraBold', 'rockwell', 'avenirHeavy']), W - 100, 104, 40, 2, upper=True)
        draw_lines(d, lines, f, 50, H - band + 44, (255, 255, 255))
        fs = font('avenir', 24)
        d.text((50, H - 70), ('  ·  '.join(names[:2] + [year])).upper(), font=fs, fill=pal[2])
    else:  # grain, italic serif
        img = mono(img, warm=rnd.random() < .5)
        img = grade(img, pal, 0.2, 1.1, 1.0)
        img = vignette(img, 0.5)
        img = grain(img, 26, h(key))
        img = darken_bottom(img, (0, 0, 0), 0.55, 210)
        d = ImageDraw.Draw(img)
        f, lines = fit_text(d, title, rnd.choice(['didotBold', 'baskervilleItalic', 'hoefler']), W - 100, 118, 44, 3)
        asc, desc = f.getmetrics()
        y = H - 140 - len(lines) * (asc + desc)
        draw_lines(d, lines, f, 50, y, (250, 246, 238), 'center', W - 100)
        credit_block(d, W, H - 80, (215, 210, 200), names[:2] + [year])
    if kind in ('tv', 'anime'):
        d = ImageDraw.Draw(img)
        net = item.get('network', '').upper()
        fs = font('avenirBold', 20)
        tw = d.textlength(net, font=fs)
        d.rounded_rectangle((W - tw - 70, 36, W - 36, 72), 6, fill=(0, 0, 0, 0), outline=(255, 255, 255), width=2)
        d.text((W - tw - 53, 41), net, font=fs, fill=(255, 255, 255))
    return img.resize((360, 540), Image.LANCZOS)

def poster_real(item, src):
    W, H = 360, 540
    w, hh = src.size
    if abs(w / hh - 2 / 3) < 0.08:
        return cover(src, W, H)
    bg = cover(src, W, H).filter(ImageFilter.GaussianBlur(18))
    bg = ImageEnhance.Brightness(bg).enhance(0.5)
    fg = ImageOps.contain(src, (W, H), Image.LANCZOS)
    bg.paste(fg, ((W - fg.size[0]) // 2, (H - fg.size[1]) // 2))
    return bg

def poster_fallback_real(item):
    """A real film without a Commons poster: a period title card."""
    W, H = 720, 1080
    pal = ((18, 16, 12), (150, 120, 80), (240, 228, 205))
    photo = load(f"pback-{item['id']}.jpg")
    img = mono(cover(photo, W, H), warm=True) if photo else Image.new('RGB', (W, H), pal[0])
    img = vignette(img, 0.65)
    img = grain(img, 30, h(item['id']))
    d = ImageDraw.Draw(img)
    d.rectangle((40, 40, W - 40, H - 40), outline=pal[2], width=3)
    d.rectangle((54, 54, W - 54, H - 54), outline=pal[1], width=1)
    f, lines = fit_text(d, item['title'], 'copperplate', W - 160, 96, 40, 3, upper=True)
    asc, desc = f.getmetrics()
    y = H // 2 - len(lines) * (asc + desc) // 2
    draw_lines(d, lines, f, 80, y, pal[2], 'center', W - 160)
    fs = font('baskervilleItalic', 34)
    s = str(item['year'])
    d.text(((W - d.textlength(s, font=fs)) / 2, H - 170), s, font=fs, fill=pal[2])
    names = people_names([c['person'] for c in item.get('cast', [])][:2])
    if names:
        fn = font('copperplate', 26)
        t = ' · '.join(names).upper()
        d.text(((W - d.textlength(t, font=fn)) / 2, 120), t, font=fn, fill=pal[2])
    return img.resize((360, 540), Image.LANCZOS)

# ── Backdrops and stills ─────────────────────────────────────────────────
def backdrop_for(item, tone=None):
    W, H = 1152, 648
    still = load(f"wstill-{item['id']}.jpg")
    if still is not None:
        img = cover(still, W, H)
        return ImageEnhance.Contrast(img).enhance(1.05)
    photo = load(f"pback-{item['id']}.jpg")
    if photo is None: return None
    img = cover(photo, W, H)
    tone = tone or item.get('art', {}).get('tone')
    if tone == 'mono': return grain(mono(img), 14, h(item['id']))
    if tone == 'warm': return grain(mono(img, warm=True), 12, h(item['id']))
    return grade(img, palette(item['id']), 0.28, 1.08, 0.92)

def still(i):
    p = load(f'pstill-{i}.jpg')
    if p is None: return None
    return grade(cover(p, 400, 225), PALETTES[i % len(PALETTES)], 0.18, 1.06, 0.95)

# ── Music ────────────────────────────────────────────────────────────────
def album_cover(album, artist_name):
    W = 800
    key = album['id']
    pal = palette(key)
    photo = load(f'palbum-{key}.jpg')
    style = album['art'].get('style', h(key) % 6)
    rnd = random.Random(h(key))
    title = album['title']
    if style == 0 and photo:  # photo, small label
        img = grade(cover(photo, W, W), pal, 0.3, 1.1, 0.9)
        d = ImageDraw.Draw(img)
        d.text((44, 40), artist_name.upper(), font=font('avenirBold', 30), fill=(255, 255, 255))
        d.text((44, 80), title, font=font('avenir', 30), fill=(240, 240, 240))
    elif style == 1 and photo:  # duotone, big title
        img = duotone(cover(photo, W, W), pal)
        d = ImageDraw.Draw(img)
        f, lines = fit_text(d, title, rnd.choice(['impact', 'din', 'futuraBold']), W - 100, 170, 60, 3, upper=True)
        asc, desc = f.getmetrics()
        draw_lines(d, lines, f, 50, W - 80 - len(lines) * (asc + desc), (255, 255, 255))
        d.text((52, 44), artist_name.upper(), font=font('avenirBold', 28), fill=pal[2])
    elif style == 2:  # geometric
        img = Image.new('RGB', (W, W), pal[0])
        d = ImageDraw.Draw(img, 'RGBA')
        for k in range(rnd.randint(3, 7)):
            r = rnd.randint(120, 420)
            x, y = rnd.randint(-100, W), rnd.randint(-100, W)
            col = rnd.choice([pal[1], pal[2], tuple(min(255, c + 40) for c in pal[1])])
            d.ellipse((x - r, y - r, x + r, y + r), fill=col + (rnd.randint(60, 190),))
        img = img.filter(ImageFilter.GaussianBlur(2))
        d = ImageDraw.Draw(img)
        f, lines = fit_text(d, title, 'helvBold', W - 100, 96, 40, 2)
        draw_lines(d, lines, f, 50, W - 90 - len(lines) * sum(f.getmetrics()), (255, 255, 255))
        d.text((52, 48), artist_name, font=font('helvLight', 34), fill=(255, 255, 255))
    elif style == 3:  # typographic
        img = Image.new('RGB', (W, W), pal[2] if rnd.random() < .5 else pal[1])
        ink = pal[0]
        d = ImageDraw.Draw(img)
        f, lines = fit_text(d, title, rnd.choice(['didotBold', 'bodoni', 'futuraBold', 'rockwell']), W - 120, 150, 50, 4, upper=rnd.random() < .5)
        draw_lines(d, lines, f, 60, 150, ink)
        d.line((60, W - 130, W - 60, W - 130), fill=ink, width=3)
        d.text((60, W - 110), artist_name.upper(), font=font('gill', 34), fill=ink)
    elif style == 4 and photo:  # circle crop on colour
        img = Image.new('RGB', (W, W), pal[1])
        circle = cover(grade(photo, pal, 0.3), 520, 520)
        mask = Image.new('L', (520, 520), 0)
        ImageDraw.Draw(mask).ellipse((0, 0, 520, 520), fill=255)
        img.paste(circle, (140, 90), mask)
        d = ImageDraw.Draw(img)
        t = f"{artist_name} — {title}"
        f = font('avenirBold', 32)
        d.text(((W - d.textlength(t, font=f)) / 2, W - 120), t, font=f, fill=pal[0])
    else:  # photo mono with frame
        base = cover(photo, W, W) if photo else Image.new('RGB', (W, W), pal[0])
        img = mono(base, warm=rnd.random() < .5)
        img = vignette(img, 0.4)
        d = ImageDraw.Draw(img)
        d.rectangle((30, 30, W - 30, W - 30), outline=(255, 255, 255), width=4)
        f = font('baskervilleItalic', 64)
        d.text((70, W - 170), title, font=f, fill=(255, 255, 255))
        d.text((72, 64), artist_name.upper(), font=font('gill', 30), fill=(255, 255, 255))
    return img.resize((400, 400), Image.LANCZOS)

def book_cover(book):
    W, H = 720, 1080
    key = book['id']
    pal = palette(key)
    rnd = random.Random(h(key))
    style = book['art'].get('style', h(key) % 6)
    photo = load(f'pbook-{key}.jpg')
    if style in (0, 3) and photo:
        img = duotone(cover(photo, W, H), pal)
        img = darken_bottom(img, pal[0], 0.3, 245)
    elif style == 4 and photo:
        # Title on the plain top, the photograph in a band below it.
        img = Image.new('RGB', (W, H), pal[0])
        band = cover(grade(photo, pal, 0.4), W, int(H * 0.30))
        img.paste(band, (0, int(H * 0.5)))
    else:
        img = Image.new('RGB', (W, H), tuple(int(c * 0.8) for c in pal[1]) if rnd.random() < .5 else pal[0])
        d = ImageDraw.Draw(img, 'RGBA')
        for k in range(30):
            y = rnd.randint(0, H)
            d.line((0, y, W, y + rnd.randint(-40, 40)), fill=pal[2] + (18,), width=rnd.randint(1, 3))
    d = ImageDraw.Draw(img)
    d.rectangle((36, 36, W - 36, H - 36), outline=pal[2], width=2)
    fname = rnd.choice(['baskerville', 'didot', 'hoefler', 'palatino', 'bodoni'])
    f, lines = fit_text(d, book['title'], fname, W - 150, 104 if style != 4 else 92, 44, 4 if style != 4 else 3)
    asc, desc = f.getmetrics()
    block = len(lines) * (asc + desc)
    y = min(int(H * 0.68) - block // 2, H - 200 - block) if style in (0, 3) else 150
    if style in (0, 3) and photo:
        # A soft panel behind the title so it reads over any photograph.
        panel = Image.new('RGBA', (W, H), (0, 0, 0, 0))
        ImageDraw.Draw(panel).rectangle((36, y - 30, W - 36, y + block + 44), fill=pal[0] + (200,))
        img = Image.alpha_composite(img.convert('RGBA'), panel).convert('RGB')
        d = ImageDraw.Draw(img)
        d.rectangle((36, 36, W - 36, H - 36), outline=pal[2], width=2)
    yy = draw_lines(d, lines, f, 75, y, (255, 255, 255) if style in (0, 3, 4) else pal[2], 'center', W - 150)
    d.text(((W - 60) / 2, yy + 14), '❧', font=font('hoefler', 40), fill=pal[2]) if False else None
    d.line((W / 2 - 50, yy + 34, W / 2 + 50, yy + 34), fill=pal[2], width=2)
    fa = font('copperplate', 38)
    a = book['author'].upper()
    d.text(((W - d.textlength(a, font=fa)) / 2, H - 150), a, font=fa, fill=(240, 240, 240))
    fn = font('gill', 24)
    n = f"READ BY {book['narrator'].upper()}"
    d.text(((W - d.textlength(n, font=fn)) / 2, H - 96), n, font=fn, fill=pal[2])
    return img.resize((360, 540), Image.LANCZOS)

# ── People ───────────────────────────────────────────────────────────────
def person_photo(p):
    src = load(f"wperson-{p['id']}.jpg")
    if src is not None:
        w, hh = src.size
        side = min(w, hh)
        top = 0 if hh > w else (w - side) // 2
        crop = src.crop(((w - side) // 2, max(0, int((hh - side) * 0.15)), (w - side) // 2 + side, max(0, int((hh - side) * 0.15)) + side))
        return ImageEnhance.Contrast(crop.resize((300, 300), Image.LANCZOS)).enhance(1.05)
    return None

def person_generated(p):
    """A monogram portrait (Apple Contacts style) for invented people."""
    W = 600
    key = p['id']
    pal = palette(key)
    img = Image.new('RGB', (W, W), pal[0])
    glow = Image.new('RGB', (W, W), pal[1])
    mask = Image.new('L', (W, W), 0)
    ImageDraw.Draw(mask).ellipse((-W * 0.3, -W * 0.45, W * 0.95, W * 0.7), fill=170)
    mask = mask.filter(ImageFilter.GaussianBlur(110))
    img = Image.composite(glow, img, mask)
    d = ImageDraw.Draw(img)
    parts = p['name'].split()
    initials = (parts[0][0] + (parts[-1][0] if len(parts) > 1 else '')).upper()
    f = font('didot', 250)
    tw = d.textlength(initials, font=f)
    asc, desc = f.getmetrics()
    d.text(((W - tw) / 2, (W - asc - desc) / 2 + 10), initials, font=f, fill=tuple(min(255, int(c * 0.35 + 255 * 0.65)) for c in pal[2]))
    img = grain(img, 12, h(key))
    return img.resize((300, 300), Image.LANCZOS)

# ── Channel logos ────────────────────────────────────────────────────────
def channel_logo(ch):
    W, H = 480, 270
    img = Image.new('RGBA', (W, H), (0, 0, 0, 0))
    d = ImageDraw.Draw(img)
    pal = palette(ch['id'])
    name = ch['name']
    rnd = random.Random(h(ch['id']))
    style = h(ch['id']) % 4
    if style == 0:
        d.rounded_rectangle((20, 70, 150, 200), 26, fill=pal[1])
        initials = ''.join(w[0] for w in name.split()[:2]).upper()
        f = font('avenirHeavy', 64)
        d.text((85 - d.textlength(initials, font=f) / 2, 96), initials, font=f, fill=(255, 255, 255))
        f2, lines = fit_text(d, name, 'avenirBold', 300, 52, 28, 2)
        draw_lines(d, lines, f2, 170, 135 - len(lines) * sum(f2.getmetrics()) // 2, (255, 255, 255))
    elif style == 1:
        f2, lines = fit_text(d, name, 'futuraBold', 420, 70, 30, 2, upper=True)
        yy = draw_lines(d, lines, f2, 30, 135 - len(lines) * sum(f2.getmetrics()) // 2, (255, 255, 255), 'center', 420)
        d.rectangle((W / 2 - 60, yy + 8, W / 2 + 60, yy + 16), fill=pal[1])
    elif style == 2:
        d.ellipse((24, 75, 144, 195), fill=pal[1])
        d.ellipse((54, 105, 114, 165), fill=(255, 255, 255))
        f2, lines = fit_text(d, name, 'helvBold', 300, 56, 28, 2)
        draw_lines(d, lines, f2, 168, 135 - len(lines) * sum(f2.getmetrics()) // 2, (255, 255, 255))
    else:
        num = ch['number'].split('.')[0]
        f = font('din', 150)
        tw = d.textlength(num, font=f)
        d.text((40, 50), num, font=f, fill=pal[1])
        f2, lines = fit_text(d, name, 'avenirCond', 440 - tw - 60, 58, 26, 2, upper=True)
        draw_lines(d, lines, f2, 40 + tw + 24, 135 - len(lines) * sum(f2.getmetrics()) // 2, (255, 255, 255))
    return img.resize((240, 135), Image.LANCZOS)

# ── Jobs ─────────────────────────────────────────────────────────────────
def save_jpg(img, sub, name, q=78):
    d = os.path.join(OUT, sub)
    os.makedirs(d, exist_ok=True)
    img.convert('RGB').save(os.path.join(d, name + '.jpg'), 'JPEG', quality=q, optimize=True, progressive=True)

def save_png(img, sub, name):
    d = os.path.join(OUT, sub)
    os.makedirs(d, exist_ok=True)
    img.save(os.path.join(d, name + '.png'), 'PNG', optimize=True)

def job_movie(m):
    out = {}
    if m['art']['kind'] == 'wiki':
        src = load(f"wposter-{m['id']}.jpg")
        img = poster_real(m, src) if src else poster_fallback_real(m)
    else:
        photo = load(f"pposter-{m['id']}.jpg") or load(f"pback-{m['id']}.jpg")
        img = poster_generated(m, photo) if photo else None
    if img: save_jpg(img, 'posters', m['id']); out['poster'] = True
    bd = backdrop_for(m)
    if bd: save_jpg(bd, 'backdrops', m['id'], 72); out['backdrop'] = True
    return ('movie', m['id'], out)

def job_show(s):
    out = {}
    photo = load(f"pposter-{s['id']}.jpg") or load(f"pback-{s['id']}.jpg")
    if photo:
        save_jpg(poster_generated(s, photo, s['kind']), 'posters', s['id']); out['poster'] = True
    bd = backdrop_for(s)
    if bd: save_jpg(bd, 'backdrops', s['id'], 72); out['backdrop'] = True
    return ('show', s['id'], out)

def job_still(i):
    img = still(i)
    if img: save_jpg(img, 'stills', f'st-{i}', 74); return ('still', i, True)
    return ('still', i, False)

def job_album(a):
    artist = next(x['name'] for x in plan['artists'] if x['id'] == a['artist'])
    save_jpg(album_cover(a, artist), 'albums', a['id'], 80)
    return ('album', a['id'], True)

def job_artist(a):
    photo = load(f"partist-{a['id']}.jpg")
    out = {}
    if photo:
        save_jpg(grade(cover(photo, 400, 400), palette(a['id']), 0.3, 1.1, 0.9), 'artists', a['id']); out['photo'] = True
    bg = load(f"partistbg-{a['id']}.jpg")
    if bg:
        save_jpg(grade(cover(bg, 1152, 648), palette(a['id']), 0.35, 1.05, 0.85), 'backdrops', a['id'], 70); out['backdrop'] = True
    return ('artist', a['id'], out)

def job_book(b):
    save_jpg(book_cover(b), 'books', b['id'])
    return ('book', b['id'], True)

def job_person(p):
    img = person_photo(p)
    real = img is not None
    if img is None: img = person_generated(p)
    save_jpg(img, 'people', p['id'], 80)
    return ('person', p['id'], real)

def job_photo(ph):
    src = load(f"photo-{ph['id']}.jpg")
    if src is None: return ('photo', ph['id'], False)
    full = ImageOps.contain(src, (1280, 1280), Image.LANCZOS)
    save_jpg(full, 'photos', ph['id'], 76)
    thumb = ImageOps.fit(src, (360, 360), Image.LANCZOS)
    save_jpg(thumb, 'photothumbs', ph['id'], 74)
    return ('photo', ph['id'], True)

def job_homevideo(v):
    src = load(f"phome-{v['id']}.jpg")
    if src is None: return ('homevideo', v['id'], False)
    img = cover(src, 1152, 648)
    save_jpg(img, 'backdrops', v['id'], 70)
    save_jpg(cover(src, 400, 225), 'stills', v['id'], 74)
    return ('homevideo', v['id'], True)

def job_channel(ch):
    save_png(channel_logo(ch), 'logos', ch['id'])
    return ('channel', ch['id'], True)

def run_all():
    jobs = [(job_movie, m) for m in plan['movies']] + [(job_show, s) for s in plan['shows']] + [(job_still, i) for i in range(520)] \
        + [(job_album, a) for a in plan['albums']] + [(job_artist, a) for a in plan['artists']] + [(job_book, b) for b in plan['books']] \
        + [(job_person, p) for p in plan['people']] + [(job_photo, ph) for al in plan['photoAlbums'] for ph in al['photos']] \
        + [(job_homevideo, v) for v in plan['homeVideos']] + [(job_channel, c) for c in plan['channels']]
    with ProcessPoolExecutor(8) as ex:
        results = list(ex.map(_call, jobs, chunksize=4))
    return results

def _call(job):
    fn, arg = job
    try:
        return fn(arg)
    except Exception as e:
        import traceback; traceback.print_exc()
        return ('error', str(arg.get('id') if isinstance(arg, dict) else arg), str(e))

def manifest(results):
    files = {}
    for sub in sorted(os.listdir(OUT)):
        p = os.path.join(OUT, sub)
        if not os.path.isdir(p): continue
        files[sub] = sorted(os.path.splitext(f)[0] for f in os.listdir(p) if not f.startswith('.'))
    lines = ['/* Generated by scripts/art/render.py. Do not edit. */', '/* eslint-disable */', 'import type {ImageSourcePropType} from \'react-native\';', '']
    for sub, names in files.items():
        ext = 'png' if sub == 'logos' else 'jpg'
        lines.append(f'export const {sub}: Record<string, ImageSourcePropType> = {{')
        for n in names:
            lines.append(f"  '{n}': require('../../../assets/art/{sub}/{n}.{ext}'),")
        lines.append('};')
        lines.append('')
    import colorsys
    tints = {}
    for sub in ('posters', 'albums', 'books'):
        for n in files.get(sub, []):
            im = Image.open(os.path.join(OUT, sub, n + '.jpg')).convert('RGB').resize((24, 36))
            px = list(im.getdata())
            best = max(px, key=lambda c: colorsys.rgb_to_hsv(*[x / 255 for x in c])[1] * (0.3 + colorsys.rgb_to_hsv(*[x / 255 for x in c])[2]))
            hh, ss, vv = colorsys.rgb_to_hsv(*[x / 255 for x in best])
            r, g, b = colorsys.hsv_to_rgb(hh, min(0.75, max(0.35, ss)), 0.62)
            tints[n] = '#%02x%02x%02x' % (int(r * 255), int(g * 255), int(b * 255))
    lines.append('/** A saturated accent colour per poster, for the glow tint. */')
    lines.append('export const tints: Record<string, string> = ' + json.dumps(tints) + ';')
    real_people = sorted(r[1] for r in results if r[0] == 'person' and r[2] is True)
    lines.append('/** People whose photo is a real (Commons) photograph. */')
    lines.append('export const realPhotoPeople = new Set<string>(' + json.dumps(real_people) + ');')
    out = os.path.join(ROOT, 'src', 'data', 'fixtures', 'art.generated.ts')
    open(out, 'w').write('\n'.join(lines) + '\n')
    total = sum(os.path.getsize(os.path.join(dp, f)) for dp, _, fs in os.walk(OUT) for f in fs)
    print('manifest', {k: len(v) for k, v in files.items()}, 'total MB', round(total / 1e6, 1))

if __name__ == '__main__':
    import sys
    if len(sys.argv) > 1 and sys.argv[1] == 'people':
        res = [job_person(p) for p in plan['people']]
        manifest(res)
    elif len(sys.argv) > 1 and sys.argv[1] == 'books':
        for b in plan['books']: job_book(b)
        print('book covers written')
    elif len(sys.argv) > 1 and sys.argv[1] == 'albums':
        # Covers only (after scripts/art/names.py); file names and the manifest are unchanged.
        for a in plan['albums']: job_album(a)
        print('album covers written')
    elif len(sys.argv) > 1 and sys.argv[1] == 'sample':
        os.makedirs(os.path.join(CACHE, 'sample'), exist_ok=True)
        ms = [m for m in plan['movies'] if m['art']['kind'] == 'generated'][:16]
        for i, m in enumerate(ms):
            m = dict(m); m['art'] = {'kind': 'generated', 'style': i % 8}
            photo = load(f"pposter-{m['id']}.jpg")
            if photo: poster_generated(m, photo).save(os.path.join(CACHE, 'sample', f'poster{i}.jpg'))
        sheet = Image.new('RGB', (360 * 8, 540 * 2))
        for i in range(16):
            p = os.path.join(CACHE, 'sample', f'poster{i}.jpg')
            if os.path.exists(p): sheet.paste(Image.open(p), ((i % 8) * 360, (i // 8) * 540))
        sheet.resize((360 * 4, 270 * 2)).save(os.path.join(CACHE, 'sample', 'sheet.jpg'))
        print('sample written')
    else:
        res = run_all()
        errs = [r for r in res if r[0] == 'error']
        print('errors', len(errs), errs[:5])
        manifest(res)
