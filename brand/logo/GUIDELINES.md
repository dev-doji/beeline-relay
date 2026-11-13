# Beeline logo: usage guide

**The idea.** A straight line fits a notch in a honey cell exactly, with a band of negative space around it.
That is what Beeline does: the signed invoice names one exact payee and one exact amount, and the vault
accepts nothing else. It is not a bee drawing, a shield or an arrow.

## Files (in this folder)

| Use | File |
|---|---|
| The mark (master, colour) | `beeline-symbol.svg` |
| The mark at 16-32 px, colour only | `beeline-symbol-small.svg` |
| The mark on dark backgrounds | `beeline-symbol-reversed.svg` (small: `beeline-symbol-small-reversed.svg`) |
| Horizontal logo (header, email, README) | `beeline-horizontal.svg`, reversed: `beeline-horizontal-reversed.svg` |
| Stacked logo (posters, social, signs) | `beeline-stacked.svg`, reversed: `beeline-stacked-reversed.svg` |
| Wordmark alone | `beeline-wordmark.svg`, reversed: `beeline-wordmark-reversed.svg` |
| One-colour black / white / ink, avatar square, PNGs | `kit/symbol`, `kit/horizontal`, `kit/stacked`, `kit/wordmark` |
| Favicon, app icon, home-screen icons, manifest | `kit/web` (`head-snippet.html` shows the tags) |
| Presentation board and mockups | `presentation.html`, `slides/slide-01.png` to `slide-05.png` |
| Test sheets | `preview-final.html`, `renders/` |
| Regenerate everything | `python make_kit.py`, then `python build_exports.py` |

## Colour

| Name | HEX | RGB | CMYK (plain conversion) |
|---|---|---|---|
| Honey | `#F5C451` | 245, 196, 81 | 0, 20, 67, 4 |
| Ink | `#16162A` | 22, 22, 42 | 48, 48, 0, 84 |
| White | `#FFFFFF` | 255, 255, 255 | 0, 0, 0, 0 |

The CMYK values are a plain mathematical conversion, not a press proof. Proof them before printing, and check
Pantone against a swatch book; no Pantone match has been verified.

Contrast (measured): ink on honey 10.9:1, honey on ink 10.9:1, white on ink 17.8:1, ink on white 17.8:1.
**Honey on white is only 1.63:1: never use honey for text or thin lines on white.** On white backgrounds the
honey cell is fine because the ink line and wordmark carry the contrast.

## Versions and when to use them

- **Full colour** (`beeline-symbol`): default on white, `#F7F7FA` and light photos.
- **Reversed** (`*-reversed`): on ink or other dark backgrounds. The line becomes white and is thinned from 24
  to 22 units with a wider band (6), because white shapes look heavier on dark.
- **One colour** (black, white or ink from `kit/`): for stamps, fax, engraving, embroidery. It works because the
  band around the line is real negative space, not a white shape painted on top.
- **Small cut** (`beeline-symbol-small`): 16 to 32 px only, in colour only. It has a thicker line and no band.
  **Do not use the small cut in one colour: the line closes up into a solid hexagon.** For one colour at small
  sizes use the full symbol at 24 px or larger.

## Minimum sizes

| Item | Minimum |
|---|---|
| Symbol, full | 24 px (6 mm in print) |
| Symbol, small cut, colour | 16 px |
| Horizontal logo | 96 px wide |
| Stacked logo | 64 px wide |

## Clear space

Let x be the line's thickness (24 units on the 256 unit symbol canvas, about 9% of its width). Keep 2x clear on
every side. The symbol files already contain about 1x of padding, so add one more x around them.

## Backgrounds

Approved: white, `#F7F7FA`, ink `#16162A` (use the reversed files), and a honey tile with the one-colour ink
version. Avoid busy photos, and any background close to honey with the full-colour version.

## Misuse: do not

- recolour (no blue, no gradients), add shadows, outlines or glows;
- stretch, rotate, mirror or crop the hexagon;
- close the band around the line, or move the line out of the notch;
- retype the wordmark in a font or change the honey dot on the i;
- put the full-colour logo on honey, or honey text on white;
- use the small cut above 32 px or in one colour.

## Construction (for designers)

- Symbol canvas 256 x 256. Pointy-top hexagon, circumradius 104, one corner radius (10) on every corner.
- The line is 24 high with round ends. The notch is the line plus a 5 unit band each side, ending in a
  semicircle concentric with the line's end. The whole mark is shifted 12 units right so its bounding box is
  centred (margins 26 / 26).
- Wordmark: lowercase "beeline", one monoline weight (20 units, which is 24 at the lockup's 1.2 scale, the same
  as the symbol's line), round caps, letters drawn as paths (no font). The dot of the i is a honey hexagon.
- Horizontal lockup: the wordmark's x-height midline sits on the symbol's centre line.

## Tests run (2026-11-13)

- Audit: symbol 100/100, small cut 100/100, wordmark 99, horizontal 98, stacked 97; two colours only, no
  gradients, no live text, no filters.
- Size ladder 16 / 24 / 32 / 64 px: the idea reads at 24 px and up; at 16 px it is a honey hexagon with a dark dash.
- One colour: the full symbol passes. The small cut fails by design (see above).
- Reversed: checked on ink; the thinned line holds.
- Mockups: payment card, app icon, website, social profile, business cards, browser tab (placeholder names).

## Open items (not done)

1. **Trademark and reverse-image search.** Not done. Do it before launch.
2. **Wordmark outlines.** The wordmark is built from stroked lines. Expand the strokes to outlines in
   Illustrator or Inkscape before sending it to a printer or sign maker.
3. **CMYK and Pantone** need a proof (see Colour).
4. The new mark is in the site (2026-11-13): `beeline-web/components/BeeMark.tsx` (picks the full or small cut by size
   and the reversed cut with `onDark`), plus `app/favicon.ico`, `app/icon.svg`, `app/apple-icon.png`,
   `app/manifest.webmanifest` and `public/icon-*.png`. Other places that show the logo (the README, social
   preview images, the GitHub repository avatar) are not updated.
5. The mockup slides contain placeholder text (a card holder name, `@beeline`, `beeline.com`).
