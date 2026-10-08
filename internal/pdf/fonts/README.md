# Fonts

Licences in `OFL-*.txt`, summary in `/THIRD_PARTY_NOTICES.md`. Total size about 1.0 MB.

- `BeVietnamPro-Regular.ttf`, `BeVietnamPro-Bold.ttf`: unmodified, from `google/fonts` (`ofl/bevietnampro`). Full Vietnamese coverage.
- `NotoEmoji-Regular.ttf`: monochrome emoji outlines, **derived** from the variable font in `google/fonts` (`ofl/notoemoji`)
  with fontTools (`pip install fonttools`), because the original is 2 MB and fpdf cannot read variable fonts or
  code points above U+FFFF:

```sh
fonttools varLib.instancer 'NotoEmoji[wght].ttf' wght=400 -o NotoEmoji-400.ttf
pyftsubset NotoEmoji-400.ttf --unicodes='U+0020-007E,U+00A0,U+200D,U+2190-21FF,U+2300-23FF,U+2600-27BF,U+2B00-2BFF,U+FE0F,U+1F000-1FAFF' \
  --layout-features='' --no-hinting --output-file=NotoEmoji-Regular.ttf
python3 - <<'PY'   # alias U+1F000..U+1FAFF to U+E000..U+EAFF in the BMP cmap (what fpdf reads)
from fontTools.ttLib import TTFont
f = TTFont("NotoEmoji-Regular.ttf")
for t in f["cmap"].tables:
    if t.isUnicode() and t.format == 4:
        for cp, g in list(f.getBestCmap().items()):
            if 0x1F000 <= cp <= 0x1FAFF:
                t.cmap[0xE000 + cp - 0x1F000] = g
f.save("NotoEmoji-Regular.ttf")
PY
```

The modified file keeps the name "Noto Emoji" (allowed: the OFL text declares no Reserved Font Name); it is not the upstream file.

The original supplementary-plane mappings stay in the file, so other engines still work with the real code points.

## Registry

`fonts.go` also keeps the registry of families a template may name (`Register`, `Lookup`, `Names`). To add a family:
put the TTF files and the licence text here, embed them, call `Register` from `init`, and add them to
`/THIRD_PARTY_NOTICES.md`. `Register` panics unless every face passes the Vietnamese coverage check (all letters
with every tone mark, upper and lower case), and `TestRegistryCoversVietnamese` repeats it for the whole registry.
