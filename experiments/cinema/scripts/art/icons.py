"""App icons and the tvOS top shelf for Portico Cinema, from Portico's official brand set.

Sources live in assets/brand/portico (copied from the Portico brand assets; see its README).
The artwork is used as drawn: the only change is removing the rounded corner from the icon
backgrounds, because iOS and tvOS apply their own mask and need full-bleed images.
- iOS: portico-appicon-dark (navy, white Play-P symbol), 1024 square.
- tvOS: portico-tv-appicon-wordmark-dark split into a Back layer (navy gradient) and a Front layer
  (white wordmark) so the system parallax works; top shelf images are the brand PNGs.
"""
import glob, json, os, re, shutil, subprocess

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), '..', '..'))
BRAND = os.path.join(ROOT, 'assets', 'brand', 'portico')


def svg(name):
    return open(os.path.join(BRAND, 'svg', name + '.svg')).read()


def render(text, w, h, out):
    subprocess.run(['rsvg-convert', '-w', str(w), '-h', str(h), '-o', out], input=text.encode(), check=True)


def full_bleed(text):
    return re.sub(r'(<rect[^>]*?)\s+rx="[^"]*"', r'\1', text)


def back_layer(text):
    return re.sub(r'<g\b.*?</g>', '', full_bleed(text), flags=re.S)


def front_layer(text):
    return re.sub(r'<rect[^>]*/>', '', text)


def write_ios():
    d = os.path.join(ROOT, 'ios/PorticoCinema/Images.xcassets/AppIcon.appiconset')
    out = os.path.join(d, 'icon-1024.png')
    render(full_bleed(svg('portico-appicon-dark')), 1024, 1024, out)
    subprocess.run(['python3', '-c', f"from PIL import Image; Image.open('{out}').convert('RGB').save('{out}')"], check=True)
    json.dump({'images': [{'filename': 'icon-1024.png', 'idiom': 'universal', 'platform': 'ios', 'size': '1024x1024'}], 'info': {'author': 'xcode', 'version': 1}}, open(os.path.join(d, 'Contents.json'), 'w'), indent=2)


def write_tvos():
    base = os.path.join(ROOT, 'tvos/PorticoCinema/Images.xcassets/App Icon & Top Shelf Image.brandassets')
    icon = svg('portico-tv-appicon-wordmark-dark')
    shelves = {'Top Shelf Image.imageset': ('portico-tv-topshelf-1920x720.png', 'portico-tv-topshelf-3840x1440.png'),
               'Top Shelf Image Wide.imageset': ('portico-tv-topshelf-wide-2320x720.png', 'portico-tv-topshelf-wide-4640x1440.png')}
    sizes = {'App Icon.imagestack': (400, 240), 'App Icon - App Store.imagestack': (1280, 768)}
    for cj in glob.glob(os.path.join(base, '**', 'Contents.json'), recursive=True):
        data = json.load(open(cj))
        if 'images' not in data:
            continue
        folder = os.path.relpath(cj, base).split(os.sep)[0]
        for img in data['images']:
            scale = int(img.get('scale', '1x')[0])
            name = img['filename']
            out = os.path.join(os.path.dirname(cj), name)
            if folder in shelves:
                shutil.copy(os.path.join(BRAND, 'png', shelves[folder][scale - 1]), out)
            else:
                w, h = sizes[folder]
                layer = back_layer(icon) if 'Back' in cj else front_layer(icon)
                render(layer, w * scale, h * scale, out)


if __name__ == '__main__':
    write_ios()
    write_tvos()
    print('icons written')
