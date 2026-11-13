"""Export the Beeline kit: one-colour variants, square/avatar masters, favicon and app-icon set.

Needs the skill's scripts (C:/Users/USER/.claude/skills/logo-design/scripts) and Chrome for PNGs.
Run after make_kit.py:  python build_exports.py
"""
import os
import shutil
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
SKILL = "C:/Users/USER/.claude/skills/logo-design/scripts"
EXPORT = os.path.join(SKILL, "export_variants.py")
INK = "#16162A"
KIT = os.path.join(HERE, "kit")


def run(*args):
    cmd = [sys.executable, EXPORT, *args]
    res = subprocess.run(cmd, capture_output=True, text=True)
    if res.returncode:
        print(res.stdout[-600:], res.stderr[-600:])
        raise SystemExit(f"export failed: {' '.join(args[:2])}")
    return res.stdout


def src(name):
    return os.path.join(HERE, name)


shutil.rmtree(KIT, ignore_errors=True)
os.makedirs(KIT)

# Symbol: black, white, ink, square master, favicon svg, app-icon svg, and PNGs.
run(src("beeline-symbol.svg"), "--out-dir", os.path.join(KIT, "symbol"), "--title", "Beeline logo symbol",
    "--mono", INK, "--icon-bg", INK, "--icon-fg", "keep", "--png", "64", "256", "1024")
# Lockups and wordmark: one-colour sets and PNGs.
for name, title in [("beeline-horizontal.svg", "Beeline logo"), ("beeline-stacked.svg", "Beeline logo, stacked"), ("beeline-wordmark.svg", "beeline wordmark")]:
    run(src(name), "--out-dir", os.path.join(KIT, name.replace("beeline-", "").replace(".svg", "")), "--title", title,
        "--only", "black", "white", "mono", "--mono", INK, "--png", "1200")

# Web icons. The browser-tab icons use the colour small cut; the app/home-screen icons sit on an ink tile
# and use the reversed small cut (a white line, since an ink line would vanish on ink).
tab = os.path.join(KIT, "_tab")
app = os.path.join(KIT, "_app")
run(src("beeline-symbol.svg"), "--favicon-source", src("beeline-symbol-small.svg"), "--web-icons", "--icon-bg", "#FFFFFF", "--icon-fg", "keep",
    "--out-dir", tab, "--title", "Beeline")
run(src("beeline-symbol.svg"), "--favicon-source", src("beeline-symbol-small-reversed.svg"), "--web-icons", "--icon-bg", INK, "--icon-fg", "keep",
    "--out-dir", app, "--title", "Beeline")
web = os.path.join(KIT, "web")
os.makedirs(web)
for fn in os.listdir(tab):
    if fn.startswith("favicon"):
        shutil.copy(os.path.join(tab, fn), os.path.join(web, fn))
for fn in os.listdir(app):
    if fn.startswith(("apple-touch", "icon-", "maskable", "site.webmanifest", "head-snippet")):
        shutil.copy(os.path.join(app, fn), os.path.join(web, fn))
shutil.rmtree(tab, ignore_errors=True)
shutil.rmtree(app, ignore_errors=True)
shutil.copy(src("beeline-symbol-small.svg"), os.path.join(web, "favicon.svg"))

# A flat list for the handover notes.
for root, _, files in os.walk(KIT):
    for fn in sorted(files):
        print(os.path.relpath(os.path.join(root, fn), KIT))
