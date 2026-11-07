#!/usr/bin/env python3
"""Update the EMBEDDED_HASHES block in install.sh with the hashes of the
artifacts in DIST_DIR under the given TAG.

Usage: scripts/update-embedded-hashes.py TAG DIST_DIR

Requires Python 3 (present on GitHub Actions runners). Rebuilds the block so
that older tags' entries are preserved, entries for TAG are replaced, and
output is deterministic (sorted by version descending, then os, then arch).
"""
import hashlib
import os
import re
import sys

TAG = sys.argv[1] if len(sys.argv) > 1 else ""
DIST = sys.argv[2] if len(sys.argv) > 2 else ""
if not TAG or not DIST:
    sys.exit("usage: update-embedded-hashes.py TAG DIST_DIR")

HERE = os.path.dirname(os.path.abspath(__file__))
INSTALL = os.path.normpath(os.path.join(HERE, "..", "install.sh"))

if not os.path.isfile(INSTALL):
    sys.exit(f"install.sh not found at {INSTALL}")
if not os.path.isdir(DIST):
    sys.exit(f"dist directory not found at {DIST}")

NAME_RE = re.compile(r"^llama-runner_(?P<ver>[^_]+)_(?P<os>[^_]+)_(?P<arch>[^.]+)\.(tar\.gz|zip)$")

# --- collect existing entries -------------------------------------------------
with open(INSTALL, encoding="utf-8") as f:
    content = f.read()

block_re = re.compile(r"EMBEDDED_HASHES='\n(.*?)\n'", re.DOTALL)
m = block_re.search(content)
if not m:
    sys.exit("could not find EMBEDDED_HASHES block in install.sh")

entries = {}
for line in m.group(1).split("\n"):
    if not line.strip():
        continue
    parts = line.split("\t")
    if len(parts) != 4:
        continue
    v, o, a, h = parts
    if v == TAG:
        continue  # will be replaced
    entries[(v, o, a)] = h

# --- scan dist for this tag ---------------------------------------------------
added = 0
for name in sorted(os.listdir(DIST)):
    mm = NAME_RE.match(name)
    if not mm:
        continue
    ver = mm.group("ver")
    o = mm.group("os")
    a = mm.group("arch")
    path = os.path.join(DIST, name)
    h = hashlib.sha256(open(path, "rb").read()).hexdigest()
    entries[(TAG, o, a)] = h
    added += 1
    # sanity: the artifact's embedded version should equal the tag we are
    # publishing for (skip leading 'v' and any git-describe suffix)
    expected = TAG.lstrip("v").split("-", 1)[0]
    if ver != expected:
        print(f"warning: {name} has version '{ver}' but tag is '{TAG}'", file=sys.stderr)

if added == 0:
    sys.exit(f"no llama-runner artifacts found in {DIST}")

# --- deterministic order ------------------------------------------------------
def ver_key(v):
    core = v.lstrip("v").split("-", 1)[0]
    try:
        return tuple(int(p) for p in core.split("."))
    except ValueError:
        return (0,)

ordered = sorted(entries.items(), key=lambda kv: (ver_key(kv[0][0]), kv[0][1], kv[0][2]), reverse=True)
new_block = "\n".join(f"{v}\t{o}\t{a}\t{h}" for (v, o, a), h in ordered)

new_content = content[: m.start(1)] + new_block + content[m.end(1) :]

with open(INSTALL, "w", encoding="utf-8") as f:
    f.write(new_content)
os.chmod(INSTALL, 0o755)

print(f"Updated {INSTALL}: {added} entries added for {TAG}, {len(entries)} total")