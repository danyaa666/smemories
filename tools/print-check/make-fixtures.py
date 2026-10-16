#!/usr/bin/env python3
"""Synthetic photos for spike T-054 (no real people, no real data).

  make-fixtures.py small OUT_DIR   30 JPEGs, 1600x1200 or 1200x1600, smooth (committed under web/src/spike/print/fixtures)
  make-fixtures.py large OUT_DIR   30 JPEGs, 3000x2250 or 2250x3000 with sensor-like noise (about 1 MB each, not committed;
                                   3000 px is the longest side the media pipeline stores, docs/media.md)
Needs Pillow only.
"""
import colorsys, os, random, sys
from PIL import Image, ImageChops, ImageDraw, ImageFont

mode, out = sys.argv[1], sys.argv[2]
os.makedirs(out, exist_ok=True)
long_side = 1600 if mode == "small" else 3000
quality = 72 if mode == "small" else 88

def scene(i):
    rnd = random.Random(i)
    portrait = i % 2 == 0
    w, h = (long_side * 3 // 4, long_side) if portrait else (long_side, long_side * 3 // 4)
    hue = (i * 0.071) % 1
    top = tuple(int(c * 255) for c in colorsys.hsv_to_rgb(hue, 0.35, 0.98))
    bot = tuple(int(c * 255) for c in colorsys.hsv_to_rgb((hue + 0.12) % 1, 0.55, 0.75))
    grad = Image.linear_gradient("L").resize((w, h))
    img = Image.composite(Image.new("RGB", (w, h), bot), Image.new("RGB", (w, h), top), grad)
    d = ImageDraw.Draw(img)
    for k in range(5):  # soft hills and blobs
        c = tuple(int(c * 255) for c in colorsys.hsv_to_rgb((hue + 0.3 + k * 0.05) % 1, 0.45, 0.5 + k * 0.08))
        cx, cy, r = rnd.randint(0, w), int(h * (0.55 + 0.1 * k)), rnd.randint(w // 4, w // 2)
        d.ellipse((cx - r, cy - r // 2, cx + r, cy + r // 2), fill=c)
    d.ellipse((w * 0.7, h * 0.1, w * 0.7 + h * 0.15, h * 0.1 + h * 0.15), fill=(255, 244, 200))
    f = ImageFont.load_default(size=h // 12)
    d.text((w // 20, h // 20), "PHOTO %02d" % (i + 1), fill=(255, 255, 255), font=f)
    if mode == "large":
        noise = Image.effect_noise((w, h), 9).convert("RGB")
        img = ImageChops.add(img, noise, scale=1.0, offset=-128)
    return img

for i in range(30):
    scene(i).save(os.path.join(out, "photo-%02d.jpg" % (i + 1)), quality=quality, optimize=True)
