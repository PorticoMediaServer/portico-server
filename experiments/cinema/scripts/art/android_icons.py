"""Generate Android launcher assets from the experiment's existing Portico artwork.

Uses Pillow and the checked-in brand/icon PNGs; does not touch Apple assets.
"""
from pathlib import Path

from PIL import Image


ROOT = Path(__file__).resolve().parents[2]
RES = ROOT / "android/app/src/main/res"
BRAND = ROOT / "assets/brand/portico/png"
TV_ICON = ROOT / "tvos/PorticoCinema/Images.xcassets/App Icon & Top Shelf Image.brandassets/App Icon.imagestack"


def main():
    icon = Image.open(BRAND / "portico-appicon-dark-1024.png").convert("RGBA")
    for density, size in (("mdpi", 48), ("hdpi", 72), ("xhdpi", 96), ("xxhdpi", 144), ("xxxhdpi", 192)):
        for name in ("ic_launcher.png", "ic_launcher_round.png"):
            icon.resize((size, size), Image.Resampling.LANCZOS).save(RES / f"mipmap-{density}" / name)

    back = Image.open(TV_ICON / "Back.imagestacklayer/Content.imageset/Back.png").convert("RGBA")
    front = Image.open(TV_ICON / "Front.imagestacklayer/Content.imageset/Front.png").convert("RGBA")
    Image.alpha_composite(back, front).save(RES / "drawable/tv_banner.png")


if __name__ == "__main__":
    main()
