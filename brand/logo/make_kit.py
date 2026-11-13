"""Beeline logo kit: the final symbol, its small and reversed cuts, the wordmark and the lockups.

Direction A, "Exact fit": a honey hexagon with a notch cut into its left side, and a straight ink line
that enters the notch. A band of negative space wraps the line, so the notch and the line read as an
exact match.

Construction (symbol, viewBox 0 0 256 256)
  * pointy-top hexagon, circumradius 104, centred on y = 128; one corner radius 10 on every corner
  * the line is 24 high with round ends; the notch is the line plus a 5 unit band of negative space
    on each side, ending in a semicircle concentric with the line's round end
  * the whole mark is shifted 12 units right so its bounding box is centred (margins 26 / 26)
Wordmark: lowercase "beeline" built from one 24 unit monoline (round caps), the same weight as the
symbol's line. Letters are paths, not a font. The dot of the "i" is a honey hexagon.
Colours: honey #F5C451, ink #16162A (and white on dark).

Run: python make_kit.py   (writes beeline-*.svg next to this file)
"""
import math
import os

HONEY = "#F5C451"
INK = "#16162A"
WHITE = "#FFFFFF"
OUT = os.path.dirname(os.path.abspath(__file__))
R_CORNER = 10


def f(v):
    return f"{v:.2f}".rstrip("0").rstrip(".")


def hex_points(cx, cy, r):
    return [(cx + r * math.cos(math.radians(a)), cy + r * math.sin(math.radians(a))) for a in (-90, -30, 30, 90, 150, 210)]


def rounded(points, radii=None, default=R_CORNER):
    """Closed path with an arc of radius radii.get(i, default) at every corner (convex and concave)."""
    radii = radii or {}
    n = len(points)
    segs = []
    for i in range(n):
        p0, p1, p2 = points[i - 1], points[i], points[(i + 1) % n]
        r = radii.get(i, default)
        v1 = (p0[0] - p1[0], p0[1] - p1[1])
        v2 = (p2[0] - p1[0], p2[1] - p1[1])
        l1, l2 = math.hypot(*v1), math.hypot(*v2)
        u1, u2 = (v1[0] / l1, v1[1] / l1), (v2[0] / l2, v2[1] / l2)
        ang = math.acos(max(-1.0, min(1.0, u1[0] * u2[0] + u1[1] * u2[1])))
        d = r / math.tan(ang / 2)
        a = (p1[0] + u1[0] * d, p1[1] + u1[1] * d)
        b = (p1[0] + u2[0] * d, p1[1] + u2[1] * d)
        cross = (p1[0] - p0[0]) * (p2[1] - p1[1]) - (p1[1] - p0[1]) * (p2[0] - p1[0])
        segs.append((a, b, r, 1 if cross > 0 else 0))
    d = f"M{f(segs[0][0][0])} {f(segs[0][0][1])}"
    for i, (a, b, r, sweep) in enumerate(segs):
        if i:
            d += f"L{f(a[0])} {f(a[1])}"
        d += f"A{f(r)} {f(r)} 0 0 {sweep} {f(b[0])} {f(b[1])}"
    return d + "Z"


def bar(x1, x2, cy, h):
    r = h / 2
    return f"M{f(x1 + r)} {f(cy - r)}H{f(x2 - r)}A{f(r)} {f(r)} 0 0 1 {f(x2 - r)} {f(cy + r)}H{f(x1 + r)}A{f(r)} {f(r)} 0 0 1 {f(x1 + r)} {f(cy - r)}Z"


def symbol(cell, line, line_h=24, gap=5, radius=104, shift=12, x1=14):
    """The mark as SVG elements (no wrapper). cell/line are fill colours."""
    H = hex_points(128, 128, radius)
    left_x = H[5][0]
    nh = line_h / 2 + gap
    x2 = 128
    end_x = x2 - line_h / 2 + nh
    pts = [H[0], H[1], H[2], H[3], H[4], (left_x, 128 + nh), (end_x, 128 + nh), (end_x, 128 - nh), (left_x, 128 - nh), H[5]]
    cell_path = rounded(pts, {6: nh, 7: nh})
    return (f'<g transform="translate({shift} 0)"><path fill="{cell}" d="{cell_path}"/>'
            f'<path fill="{line}" d="{bar(x1, x2, 128, line_h)}"/></g>')


# ---------------------------------------------------------------- wordmark
W = 20          # monoline weight: 20 x the 1.2 lockup scale = 24, the same as the symbol's line
XH = 96         # x-height (outer)
ASC = 150       # ascender height (outer)
BASE = 170      # baseline (outer bottom edge)
cy = BASE - XH / 2          # centre of the x-height band = bowl centre
r = (XH - W) / 2            # centreline radius of round letters
y_bot = BASE - W / 2        # bottom of stems (centreline, cap adds W/2)
y_x = BASE - XH + W / 2     # top of x-height stems (centreline)
y_asc = BASE - ASC + W / 2  # top of ascender stems (centreline)
x0 = W / 2                  # left stem centreline inside a letter box
box = 2 * r + W             # outer width of a round letter (96)
DOT_R = 16


def letter_b(x, col):
    # the stem ends inside the bowl's wall, so there is no notch where they meet
    return (f'<path d="M{f(x + x0)} {f(y_asc)}V{f(cy)}" />'
            f'<circle cx="{f(x + x0 + r)}" cy="{f(cy)}" r="{f(r)}" />'), box


def letter_e(x, col):
    cx = x + x0 + r
    # bar left to right, then the loop runs counterclockwise (over the top, down the left, along the bottom)
    # and stops below the bar's right end: that gap is the e's aperture. 62 degrees keeps it open at a 24 unit weight.
    ex, ey = cx + r * math.cos(math.radians(62)), cy + r * math.sin(math.radians(62))
    return (f'<path d="M{f(cx - r)} {f(cy)}H{f(cx + r)}A{f(r)} {f(r)} 0 1 0 {f(ex)} {f(ey)}" />'), box


def letter_l(x, col):
    return f'<path d="M{f(x + x0)} {f(y_asc)}V{f(y_bot)}" />', W


def letter_i(x, col):
    hx, hy = x + x0, y_asc + 14
    dot = rounded(hex_points(hx, hy, DOT_R), default=3)
    return f'<path d="M{f(x + x0)} {f(y_x)}V{f(y_bot)}" /><path class="dot" stroke="none" fill="{col}" d="{dot}" />', W


def letter_n(x, col):
    return (f'<path d="M{f(x + x0)} {f(y_x)}V{f(y_bot)}M{f(x + x0)} {f(cy)}A{f(r)} {f(r)} 0 0 1 {f(x + x0 + 2 * r)} {f(cy)}V{f(y_bot)}" />'), box


# spacing between letters: round-round tightest, straight-straight widest
LETTERS = [(letter_b, 12), (letter_e, 12), (letter_e, 16), (letter_l, 20), (letter_i, 20), (letter_n, 14), (letter_e, 0)]


def wordmark(ink, dot):
    """Wordmark elements and its (width, height). The stroke colour is `ink`; the i-dot is `dot`."""
    x = 0
    body = []
    dot_el = ""
    for fn, gap in LETTERS:
        svg_el, adv = fn(x, dot)
        if 'class="dot"' in svg_el:
            parts = svg_el.split('<path class="dot"')
            body.append(parts[0])
            dot_el = '<path class="dot"' + parts[1]
        else:
            body.append(svg_el)
        x += adv + gap
    strokes = "".join(body)
    g = (f'<g fill="none" stroke="{ink}" stroke-width="{W}" stroke-linecap="round" stroke-linejoin="round">{strokes}</g>' + dot_el)
    return g, x, BASE


def wrap(body, w, h, title):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {f(w)} {f(h)}" role="img" aria-label="{title}">'
            f"<title>{title}</title>{body}</svg>\n")


def save(name, body, w, h, title):
    with open(os.path.join(OUT, name), "w", encoding="utf-8", newline="\n") as fh:
        fh.write(wrap(body, w, h, title))


# ---------------------------------------------------------------- symbol variants
save("beeline-symbol.svg", symbol(HONEY, INK), 256, 256, "Beeline logo symbol")
# Reversed (on ink): white geometry looks heavier, so the line is thinned from 24 to 22 and the band widened to 6.
save("beeline-symbol-reversed.svg", symbol(HONEY, WHITE, line_h=22, gap=6), 256, 256, "Beeline logo symbol, reversed")
# Small cut for 16-32 px: bigger cell, thicker line, no hairline band (it would close up at 16 px).
save("beeline-symbol-small.svg", symbol(HONEY, INK, line_h=40, gap=0, radius=118, shift=10, x1=4), 256, 256, "Beeline logo symbol, small-size cut")
save("beeline-symbol-small-reversed.svg", symbol(HONEY, WHITE, line_h=36, gap=0, radius=118, shift=10, x1=4), 256, 256, "Beeline logo symbol, small-size cut, reversed")

# ---------------------------------------------------------------- wordmark
wm, wm_w, wm_h = wordmark(INK, HONEY)
save("beeline-wordmark.svg", f'<g transform="translate(24 24)">{wm}</g>', wm_w + 48, wm_h + 48, "beeline wordmark")
wm_r, _, _ = wordmark(WHITE, HONEY)
save("beeline-wordmark-reversed.svg", f'<g transform="translate(24 24)">{wm_r}</g>', wm_w + 48, wm_h + 48, "beeline wordmark, reversed")

# ---------------------------------------------------------------- lockups
S = 1.2                              # wordmark scale in the horizontal lockup
mid_local = cy                       # y of the x-height midline in wordmark coordinates
ty = 128 - mid_local * S             # put that midline on the symbol's centre line
hx = 256 + 4                         # the symbol canvas already has a 26 unit margin on its right
total_w = hx + wm_w * S + 26


def horizontal(sym, wmk):
    return f'{sym}<g transform="translate({f(hx)} {f(ty)}) scale({S})">{wmk}</g>'


save("beeline-horizontal.svg", horizontal(symbol(HONEY, INK), wm), total_w, 256, "Beeline logo")
save("beeline-horizontal-reversed.svg", horizontal(symbol(HONEY, WHITE, line_h=22, gap=6), wm_r), total_w, 256, "Beeline logo, reversed")

S2 = 0.95
st_w = max(256, wm_w * S2) + 80
sym_x = (st_w - 256) / 2
wm_x = (st_w - wm_w * S2) / 2
st_h = 256 + 24 + wm_h * S2 + 30


def stacked(sym, wmk):
    return f'<g transform="translate({f(sym_x)} 0)">{sym}</g><g transform="translate({f(wm_x)} {f(256 + 8)}) scale({S2})">{wmk}</g>'


save("beeline-stacked.svg", stacked(symbol(HONEY, INK), wm), st_w, st_h, "Beeline logo, stacked")
save("beeline-stacked-reversed.svg", stacked(symbol(HONEY, WHITE, line_h=22, gap=6), wm_r), st_w, st_h, "Beeline logo, stacked, reversed")


# ---------------------------------------------------------------- colour facts
def lum(h):
    h = h.lstrip("#")
    c = [int(h[i:i + 2], 16) / 255 for i in (0, 2, 4)]
    c = [v / 12.92 if v <= 0.03928 else ((v + 0.055) / 1.055) ** 2.4 for v in c]
    return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2]


def contrast(a, b):
    la, lb = lum(a), lum(b)
    hi, lo = max(la, lb), min(la, lb)
    return (hi + 0.05) / (lo + 0.05)


print("contrast  ink on honey  %.1f:1" % contrast(INK, HONEY))
print("contrast  honey on ink  %.1f:1" % contrast(HONEY, INK))
print("contrast  white on ink  %.1f:1" % contrast(WHITE, INK))
print("contrast  honey on white %.2f:1  (never use honey for text on white)" % contrast(HONEY, WHITE))
print("contrast  ink on white  %.1f:1" % contrast(INK, WHITE))
print("wrote kit SVGs to", OUT)
