"""Beeline logo concepts: six marks, each a hexagon and one straight line.

Construction (viewBox 0 0 256 256, centre 128,128):
  * pointy-top hexagon, circumradius R = 104 unless noted (flat sides left/right, apothem R*cos30)
  * one corner radius for every corner of every shape: 10
  * one line weight family: 16 (line) and 28 (ring wall)
  * colours: honey #F5C451, ink #16162A. Nothing else.
Run: python make_concepts.py   (writes concept-*.svg next to this file)
"""
import math
import os

HONEY = "#F5C451"
INK = "#16162A"
R_CORNER = 10
OUT = os.path.dirname(os.path.abspath(__file__))


def hex_points(cx, cy, r):
    """Pointy-top hexagon vertices, clockwise from the top."""
    return [(cx + r * math.cos(math.radians(a)), cy + r * math.sin(math.radians(a))) for a in (-90, -30, 30, 90, 150, 210)]


def rounded(points, r=R_CORNER):
    """Closed path through points with every corner rounded by an arc of radius r (works for concave corners)."""
    n = len(points)
    segs = []
    for i in range(n):
        p0, p1, p2 = points[i - 1], points[i], points[(i + 1) % n]
        v1 = (p0[0] - p1[0], p0[1] - p1[1])
        v2 = (p2[0] - p1[0], p2[1] - p1[1])
        l1, l2 = math.hypot(*v1), math.hypot(*v2)
        u1, u2 = (v1[0] / l1, v1[1] / l1), (v2[0] / l2, v2[1] / l2)
        cosang = max(-1.0, min(1.0, u1[0] * u2[0] + u1[1] * u2[1]))
        ang = math.acos(cosang)
        d = r / math.tan(ang / 2)
        a = (p1[0] + u1[0] * d, p1[1] + u1[1] * d)
        b = (p1[0] + u2[0] * d, p1[1] + u2[1] * d)
        cross = (p1[0] - p0[0]) * (p2[1] - p1[1]) - (p1[1] - p0[1]) * (p2[0] - p1[0])
        segs.append((a, b, 1 if cross > 0 else 0))
    f = lambda v: f"{v:.2f}".rstrip("0").rstrip(".")
    d = f"M{f(segs[0][0][0])} {f(segs[0][0][1])}"
    for i, (a, b, sweep) in enumerate(segs):
        if i:
            d += f"L{f(a[0])} {f(a[1])}"
        d += f"A{r} {r} 0 0 {sweep} {f(b[0])} {f(b[1])}"
    return d + "Z"


def bar(x1, x2, cy, h):
    """Straight horizontal line with round ends, as a filled shape."""
    r = h / 2
    return f"M{x1 + r} {cy - r}H{x2 - r}A{r} {r} 0 0 1 {x2 - r} {cy + r}H{x1 + r}A{r} {r} 0 0 1 {x1 + r} {cy - r}Z"


def svg(body, title):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" role="img" aria-label="{title}">'
            f"<title>{title}</title>{body}</svg>\n")


def save(name, body, title):
    with open(os.path.join(OUT, name), "w", encoding="utf-8", newline="\n") as fh:
        fh.write(svg(body, title))


R = 104
H = hex_points(128, 128, R)
left_x = H[5][0]  # x of the flat left side

# 1. Exact fit: a notch is cut into the hexagon and the line fits it exactly (negative space).
def exact_fit(name, line_h, gap, title, line_x1=14, line_x2=128):
    """Hexagon with a notch cut into its left side; an ink line of height line_h enters it.
    gap = 0 means the notch is exactly as wide as the line (the line fits it exactly).
    gap > 0 leaves that much negative space on each side of the line (concentric round end)."""
    nh = line_h / 2 + gap  # notch half-height
    end_x = line_x2 - line_h / 2 + nh  # the notch end is concentric with the line's round end
    # notch: straight sides to end_x, then a semicircle of radius nh (built by rounding two 90-degree corners)
    pts = [H[0], H[1], H[2], H[3], H[4], (left_x, 128 + nh), (end_x, 128 + nh), (end_x, 128 - nh), (left_x, 128 - nh), H[5]]
    radii = None
    path = rounded_mixed(pts, {6: nh, 7: nh})
    save(name,
         f'<g transform="translate(12 0)"><path fill="{HONEY}" d="{path}"/><path fill="{INK}" d="{bar(line_x1, line_x2, 128, line_h)}"/></g>',
         title)


def rounded_mixed(points, special):
    """Like rounded(), but vertices whose index is in `special` use their own radius."""
    n = len(points)
    segs = []
    for i in range(n):
        p0, p1, p2 = points[i - 1], points[i], points[(i + 1) % n]
        r = special.get(i, R_CORNER)
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
    f = lambda v: f"{v:.2f}".rstrip("0").rstrip(".")
    d = f"M{f(segs[0][0][0])} {f(segs[0][0][1])}"
    for i, (a, b, r, sweep) in enumerate(segs):
        if i:
            d += f"L{f(a[0])} {f(a[1])}"
        d += f"A{f(r)} {f(r)} 0 0 {sweep} {f(b[0])} {f(b[1])}"
    return d + "Z"


exact_fit("concept-1a-exact-fit.svg", 24, 0, "Beeline mark: a line that fits a notch in a hexagon exactly")
exact_fit("concept-1b-exact-fit-gap.svg", 24, 5, "Beeline mark: a line entering a notch in a hexagon, outlined by negative space")

# 2. Flight: one straight trail line behind a hexagon travelling right.
r2 = 76
cx2 = 256 - 24 - r2 * math.cos(math.radians(30)) - 4  # right edge sits 28 from the canvas edge
H2 = hex_points(cx2, 128, r2)
left2 = H2[5][0]
total_left, total_right = 28, H2[1][0]
shift = (256 - (total_left + total_right)) / 2 - total_left + total_left  # centre the pair
shift = (256 - (total_right - total_left)) / 2 - total_left
save("concept-2-flight.svg",
     f'<g transform="translate({shift:.2f} 0)"><path fill="{INK}" d="{bar(28, left2 - 22, 128, 20)}"/><path fill="{HONEY}" d="{rounded(H2)}"/></g>',
     "Beeline mark: a hexagon with a straight flight line behind it")

# 3. Gate: a hexagon ring with one opening, and the line enters through it to the centre.
ring_out = hex_points(128, 128, 104)
ring_in = hex_points(128, 128, 62 / math.cos(math.radians(30)))  # wall thickness 28 on the flats
gap_top, gap_bot = 128 - 24, 128 + 24
ring = (rounded(ring_out) + rounded(ring_in[::-1]))
# cut the gap through the left wall by drawing the ring as an open polygon instead
wall = 28
out_pts = ring_out
in_pts = ring_in
open_poly = [out_pts[0], out_pts[1], out_pts[2], out_pts[3], out_pts[4], (out_pts[4][0], gap_bot),
             (in_pts[4][0], gap_bot), in_pts[4], in_pts[3], in_pts[2], in_pts[1], in_pts[0], in_pts[5],
             (in_pts[5][0], gap_top), (out_pts[5][0], gap_top), out_pts[5]]
save("concept-3-gate.svg",
     f'<path fill="{HONEY}" d="{rounded(open_poly)}"/><path fill="{INK}" d="{bar(14, 128 + 14, 128, 16)}"/>',
     "Beeline mark: a hexagon cell with a gate and a line entering it")

# 4. Sealed: the cell split along its long diagonal by one straight gap; honey above, ink below.
a, b = H[5], H[2]  # upper-left to lower-right vertex: the long diagonal
gap = 12
dx, dy = b[0] - a[0], b[1] - a[1]
L = math.hypot(dx, dy)
nx, ny = -dy / L, dx / L  # unit normal
off = gap / 2
top_poly = [H[0], H[1], H[2], (H[2][0] + nx * off * 0, H[2][1])]  # placeholder, replaced below
# Upper piece: top vertex, upper-right vertex, lower-right vertex, then back along the diagonal to upper-left vertex
upper = [H[0], H[1], (b[0] + nx * off, b[1] + ny * off), (a[0] + nx * off, a[1] + ny * off)]
lower = [(a[0] - nx * off, a[1] - ny * off), (b[0] - nx * off, b[1] - ny * off), H[3], H[4]]
# the diagonal runs vertex to vertex, so offset points stay on the hexagon edges: trim to the edge lines
def on_edge(pt, p, q):
    return pt
save("concept-4-sealed.svg",
     f'<path fill="{HONEY}" d="{rounded([H[0], H[1], (b[0] + nx * off, b[1] + ny * off), (a[0] + nx * off, a[1] + ny * off), H[5]])}"/>'
     f'<path fill="{INK}" d="{rounded([(a[0] - nx * off, a[1] - ny * off), (b[0] - nx * off, b[1] - ny * off), H[2], H[3], H[4]])}"/>',
     "Beeline mark: a sealed hexagon cell split by one straight line")

# 5. Stripe: one ink stripe across the cell, the bee body reduced to its essence.
band_h = 44
clip_id = "cell"
save("concept-5-stripe.svg",
     f'<defs><clipPath id="{clip_id}"><path d="{rounded(H)}"/></clipPath></defs>'
     f'<path fill="{HONEY}" d="{rounded(H)}"/><rect clip-path="url(#{clip_id})" x="0" y="{128 - band_h / 2}" width="256" height="{band_h}" fill="{INK}"/>',
     "Beeline mark: a honey hexagon with one ink stripe")

# 6. One line: a single stroke that draws the cell and leaves it as a straight tail.
pts = hex_points(112, 128, 84)
start = pts[1]  # upper-right vertex
path = "M{:.2f} {:.2f}".format(*start)
for p in [pts[2], pts[3], pts[4], pts[5], pts[0], pts[1]]:
    path += "L{:.2f} {:.2f}".format(*p)
path += "L{:.2f} {:.2f}".format(236, pts[1][1])
save("concept-6-one-line.svg",
     f'<path d="{path}" fill="none" stroke="{INK}" stroke-width="20" stroke-linecap="round" stroke-linejoin="round"/>',
     "Beeline mark: one continuous line drawing a hexagon and leaving as a straight tail")
print("wrote 6 concepts to", OUT)
