"""Shared deterministic original fixture primitives (Pillow and FFmpeg)."""
from pathlib import Path
import hashlib, math, os, shutil, subprocess, textwrap
import xml.etree.ElementTree as ET
from PIL import Image, ImageDraw, ImageFont


def run(args):
    """Run a checked media command with bounded CPU use and useful failures."""
    args = [str(a) for a in args]
    if Path(args[0]).name == 'ffmpeg':
        args[1:1] = ['-hide_banner', '-loglevel', 'error', '-nostdin', '-threads', '2', '-filter_threads', '1']
    subprocess.run(args, check=True)


def link(src, dst):
    src, dst = Path(src), Path(dst)
    dst.parent.mkdir(parents=True, exist_ok=True)
    if dst.exists():
        return
    try:
        os.link(src, dst)
    except OSError:
        shutil.copy2(src, dst)


def nfo(path, root_tag, fields):
    root = ET.Element(root_tag)
    for key, value in fields.items():
        for v in value if isinstance(value, list) else [value]:
            child = ET.SubElement(root, key)
            if isinstance(v, dict):
                for k, val in v.items():
                    ET.SubElement(child, k).text = str(val)
            elif v is not None:
                child.text = str(v)
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    ET.indent(root)
    path.write_bytes(ET.tostring(root, encoding='utf-8', xml_declaration=True))


def art(path, title, size=(600, 900), seed=0, transparent=False):
    """Original geometric print artwork; never fetch remote art or fonts."""
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    digest = hashlib.sha256((str(seed) + title).encode()).digest()
    w, h = size
    bg = tuple(18 + x % 45 for x in digest[:3])
    im = Image.new('RGBA' if transparent else 'RGB', size, (0, 0, 0, 0) if transparent else bg)
    draw = ImageDraw.Draw(im)
    accent = tuple(100 + x % 130 for x in digest[3:6])
    font_path = '/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf'
    font = ImageFont.truetype(font_path, max(18, int(w * .067)))
    small = ImageFont.truetype(font_path, max(12, int(w * .023)))
    if not transparent:
        for j in range(7):
            x = (digest[j] / 255) * w
            r = w * (.18 + j * .018)
            y = h * (.16 + j * .065)
            draw.ellipse((x-r, y-r, x+r, y+r), outline=accent, width=max(2,w//100))
        draw.rectangle((int(w*.075), int(h*.70), int(w*.925), int(h*.705)), fill=accent)
    lines = textwrap.wrap(title, width=22)
    y = h * (.34 if transparent else .75)
    for line in lines[:5]:
        draw.text((w*.075,y), line, font=font, fill=(245,241,228), stroke_width=0)
        y += font.size * 1.15
    if not transparent:
        draw.text((w*.075,h*.95), 'PORTICO  /  ORIGINAL SYNTHETIC LIBRARY',font=small,fill=accent)
    if path.suffix.lower() in ('.jpg','.jpeg'):
        im.convert('RGB').save(path, quality=88, optimize=True)
    else:
        im.save(path, optimize=True)
    return path
