#!/usr/bin/env python3
"""Renders the app icons and tvOS brand assets from the Portico brand set.

The masters live in assets/brand (copied from Portico's Brand Assets). They
are rendered as they are: no recolouring, stretching or effects.

iOS: one 1024 px icon from the square dark icon (Xcode derives the rest).
tvOS: a two-layer parallax icon made from the wordmark app icon (back: its
navy gradient, full bleed; front: the wordmark), the App Store icon, and the
top shelf images.
Needs rsvg-convert and Pillow. Run from the project root:
    python3 scripts/icons.py
"""
import json, os, re, subprocess, tempfile
from PIL import Image

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BRAND = os.path.join(ROOT, 'assets', 'brand')
INFO = {'author': 'xcode', 'version': 1}


def render(svg_text, w, h):
    with tempfile.TemporaryDirectory() as d:
        src, out = os.path.join(d, 'a.svg'), os.path.join(d, 'a.png')
        open(src, 'w').write(svg_text)
        subprocess.run(['rsvg-convert', '-w', str(w), '-h', str(h), src, '-o', out], check=True)
        return Image.open(out).convert('RGBA')


def master(name):
    return open(os.path.join(BRAND, name)).read()


def contents(path, data):
    os.makedirs(path, exist_ok=True)
    json.dump(data, open(os.path.join(path, 'Contents.json'), 'w'), indent=2)


# iOS: the square dark icon, full bleed (iOS applies its own mask).
ios_set = os.path.join(ROOT, 'ios', 'PorticoClassic', 'Images.xcassets', 'AppIcon.appiconset')
for f in os.listdir(ios_set):
    if f.endswith('.png'):
        os.remove(os.path.join(ios_set, f))
render(master('portico-square-dark.svg'), 1024, 1024).convert('RGB').save(os.path.join(ios_set, 'icon-1024.png'))
contents(ios_set, {'images': [{'filename': 'icon-1024.png', 'idiom': 'universal', 'platform': 'ios', 'size': '1024x1024'}], 'info': INFO})

# tvOS brand assets.
xc = os.path.join(ROOT, 'tvos', 'PorticoClassic', 'Images.xcassets')
brand = os.path.join(xc, 'App Icon & Top Shelf Image.brandassets')
contents(brand, {'assets': [
    {'filename': 'App Icon.imagestack', 'idiom': 'tv', 'role': 'primary-app-icon', 'size': '400x240'},
    {'filename': 'App Icon - App Store.imagestack', 'idiom': 'tv', 'role': 'primary-app-icon', 'size': '1280x768'},
    {'filename': 'Top Shelf Image.imageset', 'idiom': 'tv', 'role': 'top-shelf-image', 'size': '1920x720'},
    {'filename': 'Top Shelf Image Wide.imageset', 'idiom': 'tv', 'role': 'top-shelf-image-wide', 'size': '2320x720'},
], 'info': INFO})

TV_ICON = master('portico-tv-appicon-wordmark-dark.svg')
# tvOS rounds the corners itself, so the back layer is the gradient full bleed.
BACK = re.sub(r'<g .*?</g>', '', re.sub(r' rx="\d+"', '', TV_ICON), flags=re.S)
FRONT = re.sub(r'<rect [^>]*/>', '', TV_ICON)

for name, sizes in (('App Icon.imagestack', [(400, 240, '1x'), (800, 480, '2x')]), ('App Icon - App Store.imagestack', [(1280, 768, None)])):
    stack = os.path.join(brand, name)
    contents(stack, {'layers': [{'filename': 'Front.imagestacklayer'}, {'filename': 'Back.imagestacklayer'}], 'info': INFO})
    for layer, svg in (('Front', FRONT), ('Back', BACK)):
        contents(os.path.join(stack, f'{layer}.imagestacklayer'), {'info': INFO})
        folder = os.path.join(stack, f'{layer}.imagestacklayer', 'Content.imageset')
        os.makedirs(folder, exist_ok=True)
        for f in os.listdir(folder):
            if f.endswith('.png'):
                os.remove(os.path.join(folder, f))
        images = []
        for w, h, scale in sizes:
            img = render(svg, w, h)
            fname = f'{layer}{"@2x" if scale == "2x" else ""}.png'
            (img if layer == 'Front' else img.convert('RGB')).save(os.path.join(folder, fname))
            entry = {'filename': fname, 'idiom': 'tv'}
            if scale:
                entry['scale'] = scale
            images.append(entry)
        contents(folder, {'images': images, 'info': INFO})

for name, svg, (w, h) in (('Top Shelf Image.imageset', 'portico-tv-topshelf.svg', (1920, 720)), ('Top Shelf Image Wide.imageset', 'portico-tv-topshelf-wide.svg', (2320, 720))):
    path = os.path.join(brand, name)
    os.makedirs(path, exist_ok=True)
    render(master(svg), w, h).convert('RGB').save(os.path.join(path, 'shelf.png'))
    render(master(svg), w * 2, h * 2).convert('RGB').save(os.path.join(path, 'shelf@2x.png'))
    contents(path, {'images': [{'filename': 'shelf.png', 'idiom': 'tv', 'scale': '1x'}, {'filename': 'shelf@2x.png', 'idiom': 'tv', 'scale': '2x'}], 'info': INFO})

# Launch screens: the wordmark, centred, at the size the JavaScript splash uses.
for plat, h, scales in (('ios', 48, (1, 2, 3)), ('tvos', 84, (1, 2))):
    folder = os.path.join(ROOT, plat, 'PorticoClassic', 'Images.xcassets', 'LaunchWordmark.imageset')
    os.makedirs(folder, exist_ok=True)
    w = round(h * 1774 / 405)
    images = []
    for sc in scales:
        fname = f'wordmark@{sc}x.png'
        render(master('portico-wordmark-dark.svg'), w * sc, h * sc).save(os.path.join(folder, fname))
        images.append({'filename': fname, 'idiom': 'universal', 'scale': f'{sc}x'})
    contents(folder, {'images': images, 'info': INFO})

print('icons written')
