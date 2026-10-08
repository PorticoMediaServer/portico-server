#!/usr/bin/env python3
"""Gives every title its own backdrop.

picsum.photos seeds collide, so render.py can give two unrelated titles the
same photograph, which shows when they sit next to each other in a row or
rotate through the hero. This keeps the first title of each group of
look-alike backdrops (and any real film still) and re-renders the others from
source photos in .art-cache/src that no backdrop uses yet, preferring photos
with plenty of detail. Almost-blank photographs are replaced the same way.
Run after render.py:  python3 scripts/art/dedupe_backdrops.py
"""
import os, sys
from PIL import Image, ImageFilter, ImageStat

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import render  # noqa: E402  (palette, grade, cover, save_jpg, SRC, OUT)

BD = os.path.join(render.OUT, 'backdrops')


def sig(img):
    return list(img.convert('L').resize((32, 18)).tobytes())


def mad(a, b):
    return sum(abs(x - y) for x, y in zip(a, b)) / len(a)


def detail(img):
    g = img.convert('L').resize((160, 90))
    return ImageStat.Stat(g).stddev[0] + 2 * ImageStat.Stat(g.filter(ImageFilter.FIND_EDGES)).mean[0]


names = sorted(f[:-4] for f in os.listdir(BD) if f.endswith('.jpg'))
imgs = {n: Image.open(os.path.join(BD, n + '.jpg')).convert('RGB') for n in names}
sigs = {n: sig(im) for n, im in imgs.items()}
real = {n for n in names if os.path.exists(os.path.join(render.SRC, f'wstill-{n}.jpg'))}

# Titles to re-render: all but one of each look-alike group, and near-blank ones.
redo, kept = [], []
for n in sorted(names, key=lambda n: (n not in real, n)):
    if detail(imgs[n]) < 25 or any(mad(sigs[n], sigs[k]) < 6 for k in kept):
        redo.append(n)
    else:
        kept.append(n)

# Candidate photographs: landscape sources, detailed, unlike every kept backdrop.
used = [sigs[k] for k in kept]
pool = []
for f in sorted(os.listdir(render.SRC)):
    if not f.startswith(('pback-', 'pstill-', 'photo-', 'pposter-')):
        continue
    try:
        src = Image.open(os.path.join(render.SRC, f)).convert('RGB')
    except Exception:
        continue
    if src.width < src.height * 1.2 or src.width < 900:
        continue
    c = render.cover(src, 1152, 648)
    if detail(c) < 45:
        continue
    pool.append((f, c, sig(c)))

replaced = 0
for n in redo:
    for i, (f, c, s) in enumerate(pool):
        if all(mad(s, u) >= 12 for u in used):
            render.save_jpg(render.grade(c, render.palette(n), 0.28, 1.08, 0.92), 'backdrops', n, 72)
            used.append(s)
            pool.pop(i)
            replaced += 1
            break
    else:
        print('no unused photograph left for', n)

print(f'{len(names)} backdrops, {replaced} re-rendered')

# Episode stills: list the distinct ones, so a show never repeats a picture
# (catalog.ts steps through this list). Stills that match a home video's
# picture are left out too.
import json  # noqa: E402
ST = os.path.join(render.OUT, 'stills')
still_sigs = {f[:-4]: sig(Image.open(os.path.join(ST, f))) for f in sorted(os.listdir(ST)) if f.endswith('.jpg')}
taken = [s for k, s in still_sigs.items() if k.startswith('hv-')]
unique = []
for k in sorted((k for k in still_sigs if k.startswith('st-')), key=lambda k: int(k[3:])):
    if all(mad(still_sigs[k], t) >= 8 for t in taken):
        unique.append(k)
        taken.append(still_sigs[k])
out = os.path.join(render.ROOT, 'src', 'data', 'fixtures', 'uniqueStills.json')
json.dump(unique, open(out, 'w'))
print(f'{len(unique)} distinct episode stills of {sum(1 for k in still_sigs if k.startswith("st-"))}')
