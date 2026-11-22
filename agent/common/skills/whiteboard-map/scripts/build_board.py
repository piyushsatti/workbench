#!/usr/bin/env python3
"""Build an Excalidraw scene: one named frame per subject, a mind map inside each.

Usage:
  python3 build_board.py SPEC.json OUT.excalidraw   # build + validate
  python3 build_board.py --selftest                 # assert-based checks

Spec (see examples/spec.example.json):
  {"title": str, "foot": str (optional),
   "rows": [[frame, ...], ...]            # explicit rows, or
   "frames": [frame, ...], "columns": 3   # auto-wrapped grid
  }
  frame = {"name": str, "status": str (optional, appended to the name),
           "hub": str,
           "have":  [item],   # evidence, left of hub    (blue)
           "shows": [item],   # platforms, right of hub  (purple)
           "calls": [item],   # open decisions, bottom   (orange)
           "notes": [str]}    # the user's words, bottom (yellow), never reworded
  item = "text" | ["text", "link"] | {"text":..., "link":..., "kind":...}
  A leading "!" on an evidence/platform/call item marks it private (red).

Layout: hub centred; each side column is vertically centred on the hub; every
arrow is an elbow arrow bound at both ends (fixedPoint bindings), so segments
are only horizontal or vertical and never fan out diagonally.
"""
import json, re, sys, textwrap, random, hashlib, os

# ---- layout constants (rationale in SKILL.md) ----
BW = 300        # child box width
HUB_W = 220     # hub box width
PAD = 40        # frame inner padding
GAP_V = 18      # gap between boxes in a column (tight = same group)
GAP_H = 40      # gap between boxes in the bottom row
HUB_GAP = 80    # horizontal gap between a side column and the hub (elbow trunk sits mid-gap)
BUS_GAP = 60    # vertical gap between side columns and the bottom row
FRAME_GAP = 180 # gap between frames (about 10x GAP_V)
TOP = 60        # frame top to first box
MAX_SIDE = 7    # items per side before we warn: split the frame
MAX_FRAMES = 9  # frames per board before we warn: split the board

COLORS = dict(ev="#a5d8ff", pl="#d0bfff", call="#ffd8a8", note="#fff3bf",
              priv="#ffc9c9", hub="#ffffff")
# Non-colour cue per kind (WCAG 1.4.1): position + stroke style.
STROKE = dict(ev="solid", pl="solid", call="solid", note="dashed", priv="dotted", hub="solid")
LEGEND = [("ev", "Evidence: what I have (left)"),
          ("pl", "Platforms: where it shows (right)"),
          ("call", "Open call: your decision (below)"),
          ("note", "Your note, verbatim (below, dashed)"),
          ("priv", "Private / not claimable (dotted)")]

B62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"


def frac_index(i):
    """Fractional-index keys in ascending order: a0..az, b00..bzz, c000.."""
    if i < 62:
        return "a" + B62[i]
    i -= 62
    if i < 62 ** 2:
        return "b" + B62[i // 62] + B62[i % 62]
    i -= 62 ** 2
    return "c" + B62[i // 3844] + B62[i // 62 % 62] + B62[i % 62]


class Board:
    def __init__(self, seed):
        self.els = []
        self.rng = random.Random(seed)
        self.warnings = []
        self.notes_in = []  # (rect id, verbatim text) for the selftest

    # ---- primitives ----
    def base(self, t, x, y, w, h, **k):
        e = dict(type=t, id=f"wm{len(self.els):04d}", x=x, y=y, width=w, height=h, angle=0,
                 strokeColor="#1e1e1e", backgroundColor="transparent", fillStyle="solid",
                 strokeWidth=1, strokeStyle="solid", roughness=1, opacity=100, groupIds=[],
                 frameId=None, roundness=None, seed=self.rng.randint(1, 2 ** 31),
                 version=1, versionNonce=self.rng.randint(1, 2 ** 31), isDeleted=False,
                 boundElements=None, updated=1, link=None, locked=False, index="")
        e.update(k)
        self.els.append(e)
        return e

    def text(self, x, y, w, h, s, size, **k):
        return self.base("text", x, y, w, h, text=s, originalText=s, fontSize=size, fontFamily=5,
                         textAlign="left", verticalAlign="top", containerId=None,
                         lineHeight=1.25, autoResize=True, **k)

    def box(self, x, y, raw, kind, w=BW, size=16, link=None):
        chars = max(10, int((w - 24) / (size * 0.54)))
        lines = []
        for para in raw.split("\n"):  # keep the user's whitespace; only insert soft wraps
            lines += textwrap.wrap(para, chars, replace_whitespace=False,
                                   drop_whitespace=True) or [""]
        shown = "\n".join(l.rstrip() for l in lines)
        h = len(lines) * size * 1.25 + 28
        r = self.base("rectangle", x, y, w, h, backgroundColor=COLORS[kind],
                      roundness={"type": 3}, link=link, strokeStyle=STROKE[kind],
                      strokeWidth=2 if kind == "hub" else 1, boundElements=[])
        t = self.base("text", x + 12, y + 14, w - 24, h - 28, text=shown, originalText=raw,
                      fontSize=size, fontFamily=5, textAlign="center" if kind == "hub" else "left",
                      verticalAlign="middle", containerId=r["id"], lineHeight=1.25, autoResize=True)
        r["boundElements"].append({"type": "text", "id": t["id"]})
        r["_kind"] = kind
        if kind == "note":
            self.notes_in.append((r["id"], raw))
        return r

    def arrow(self, a, a_fp, b, b_fp, via):
        """Elbow arrow from a (fixedPoint a_fp) to b (b_fp) through absolute waypoints `via`."""
        p0 = (a["x"] + a["width"] * a_fp[0], a["y"] + a["height"] * a_fp[1])
        p1 = (b["x"] + b["width"] * b_fp[0], b["y"] + b["height"] * b_fp[1])
        pts = [p0] + list(via) + [p1]
        clean = [pts[0]]  # drop zero-length and collinear interior points
        for p in pts[1:]:
            if abs(p[0] - clean[-1][0]) < 0.01 and abs(p[1] - clean[-1][1]) < 0.01:
                continue
            if len(clean) >= 2:
                q, r = clean[-2], clean[-1]
                if (abs(q[0] - r[0]) < 0.01 and abs(r[0] - p[0]) < 0.01) or \
                   (abs(q[1] - r[1]) < 0.01 and abs(r[1] - p[1]) < 0.01):
                    clean[-1] = p
                    continue
            clean.append(p)
        rel = [[px - p0[0], py - p0[1]] for px, py in clean]
        xs = [p[0] for p in rel]; ys = [p[1] for p in rel]

        def bind(el, fp):
            # current schema: elementId + fixedPoint + mode; focus/gap kept for older readers
            return {"elementId": el["id"], "fixedPoint": list(fp), "mode": "orbit",
                    "focus": 0, "gap": 1}

        e = self.base("arrow", p0[0], p0[1], max(xs) - min(xs), max(ys) - min(ys), points=rel,
                      strokeColor="#868e96", roundness=None, elbowed=True,
                      startArrowhead=None, endArrowhead="arrow",
                      startBinding=bind(a, a_fp), endBinding=bind(b, b_fp),
                      fixedSegments=None, startIsSpecial=None, endIsSpecial=None,
                      lastCommittedPoint=None)
        a["boundElements"].append({"type": "arrow", "id": e["id"]})
        b["boundElements"].append({"type": "arrow", "id": e["id"]})
        return e

    # ---- items ----
    @staticmethod
    def item(it, kind):
        link = None
        if isinstance(it, dict):
            txt, link, kind = it["text"], it.get("link"), it.get("kind", kind)
        elif isinstance(it, (list, tuple)):
            txt, link = it[0], (it[1] if len(it) > 1 else None)
        else:
            txt = it
        if kind != "note" and txt.startswith("!"):
            kind, txt = "priv", txt[1:]
        return txt, link, kind

    def column(self, x, y, items, kind):
        out = []
        for it in items:
            txt, link, k = self.item(it, kind)
            b = self.box(x, y, txt, k, link=link)
            out.append(b)
            y += b["height"] + GAP_V
        return out

    def column_height(self, items, kind):
        # measure without keeping elements
        mark = len(self.els); notes = len(self.notes_in)
        boxes = self.column(0, 0, items, kind)
        h = sum(b["height"] for b in boxes) + GAP_V * max(0, len(boxes) - 1)
        del self.els[mark:]; del self.notes_in[notes:]
        return h


def frame_width(f):
    nb = len(f.get("calls", [])) + len(f.get("notes", []))
    return max(2 * PAD + 2 * BW + 2 * HUB_GAP + HUB_W, 2 * PAD + nb * BW + max(0, nb - 1) * GAP_H)


def build_frame(B, fx, fy, f, fw):
    start = len(B.els)
    name = f["name"] + (f"  ·  {f['status']}" if f.get("status") else "")
    have, shows = f.get("have", []), f.get("shows", [])
    for side, items in (("evidence", have), ("platforms", shows)):
        if len(items) > MAX_SIDE:
            B.warnings.append(f"frame '{f['name']}': {len(items)} {side} items > {MAX_SIDE}; split the frame")
    top = fy + TOP
    lh, rh = B.column_height(have, "ev"), B.column_height(shows, "pl")
    hub_probe = B.column_height([f["hub"]], "hub")
    H = max(lh, rh, hub_probe + 40)
    cy = top + H / 2
    cx = fx + fw / 2
    lx, rx = cx - HUB_W / 2 - HUB_GAP - BW, cx + HUB_W / 2 + HUB_GAP
    L = B.column(lx, cy - lh / 2, have, "ev")
    R = B.column(rx, cy - rh / 2, shows, "pl")
    hub = B.box(cx - HUB_W / 2, 0, f["hub"], "hub", w=HUB_W, size=20)
    hub["y"] = cy - hub["height"] / 2
    B.els[-1]["y"] = hub["y"] + 14  # its text
    # side arrows: hub edge -> trunk mid-gap -> child's facing edge
    for b in L:
        tx = cx - HUB_W / 2 - HUB_GAP / 2
        by = b["y"] + b["height"] / 2
        B.arrow(hub, (0, 0.5), b, (1, 0.5), [(tx, cy), (tx, by)])
    for b in R:
        tx = cx + HUB_W / 2 + HUB_GAP / 2
        by = b["y"] + b["height"] / 2
        B.arrow(hub, (1, 0.5), b, (0, 0.5), [(tx, cy), (tx, by)])
    # bottom row: calls then notes, one row, centred; arrows drop from hub bottom to a bus line
    bottom = [(it, "call") for it in f.get("calls", [])] + [(it, "note") for it in f.get("notes", [])]
    col_bottom = max([b["y"] + b["height"] for b in L + R] + [hub["y"] + hub["height"]])
    end_y = col_bottom
    if bottom:
        row_y = col_bottom + BUS_GAP
        bus_y = col_bottom + BUS_GAP / 2
        n = len(bottom)
        row_w = n * BW + (n - 1) * GAP_H
        x = cx - row_w / 2
        for it, kind in bottom:
            txt, link, k = B.item(it, kind)
            b = B.box(x, row_y, txt, k, link=link)
            bx = b["x"] + BW / 2
            B.arrow(hub, (0.5, 1), b, (0.5, 0), [(cx, bus_y), (bx, bus_y)])
            end_y = max(end_y, b["y"] + b["height"])
            x += BW + GAP_H
    fh = end_y - fy + PAD
    fr = B.base("frame", fx, fy, fw, fh, name=name, strokeColor="#bbb")
    for e in B.els[start:-1]:
        e["frameId"] = fr["id"]
    return fh


def build(spec):
    seed = int(hashlib.sha1(json.dumps(spec, sort_keys=True).encode()).hexdigest()[:8], 16)
    B = Board(seed)
    rows = spec.get("rows")
    if rows is None:
        frames, cols = spec["frames"], int(spec.get("columns", 3))
        rows = [frames[i:i + cols] for i in range(0, len(frames), cols)]
    total = sum(len(r) for r in rows)
    if total > MAX_FRAMES:
        B.warnings.append(f"{total} frames > {MAX_FRAMES}; consider a second board")
    ncol = max(len(r) for r in rows)
    colw = [max(frame_width(r[c]) for r in rows if c < len(r)) for c in range(ncol)]
    B.text(0, -260, 2000, 45, spec["title"], 36)
    for i, (k, t) in enumerate(spec.get("legend_override") or LEGEND):
        B.box(i * 340, -190, t, k, w=320)
    y = 0
    for row in rows:
        x, hs = 0, []
        for c, f in enumerate(row):
            hs.append(build_frame(B, x + (colw[c] - frame_width(f)) / 2, y, f, frame_width(f)))
            x += colw[c] + FRAME_GAP
        y += max(hs) + FRAME_GAP
    if spec.get("foot"):
        B.text(0, y - 60, 3000, 30, spec["foot"], 18)
    for i, e in enumerate(B.els):
        e["index"] = frac_index(i)
    scene = {"type": "excalidraw", "version": 2, "source": "whiteboard-map",
             "elements": [{k: v for k, v in e.items() if not k.startswith("_")} for e in B.els],
             "appState": {"viewBackgroundColor": "#ffffff"}, "files": {}}
    return scene, B


# ---------------- validation ----------------
def _seg_hits_rect(p, q, r, inset=1.0):
    """Axis-aligned segment p-q intersects rect r's interior (shrunk by inset)?"""
    x0, y0 = r["x"] + inset, r["y"] + inset
    x1, y1 = r["x"] + r["width"] - inset, r["y"] + r["height"] - inset
    if abs(p[1] - q[1]) < 0.01:  # horizontal
        y = p[1]; a, b = sorted((p[0], q[0]))
        return y0 < y < y1 and a < x1 and b > x0
    if abs(p[0] - q[0]) < 0.01:  # vertical
        x = p[0]; a, b = sorted((p[1], q[1]))
        return x0 < x < x1 and a < y1 and b > y0
    raise AssertionError("non-orthogonal segment")


def validate(scene, notes_in=()):
    els = scene["elements"]
    by = {e["id"]: e for e in els}
    assert len(by) == len(els), "duplicate ids"
    idx = [e["index"] for e in els]
    for k in idx:
        assert re.fullmatch(r"a[0-9A-Za-z]|b[0-9A-Za-z]{2}|c[0-9A-Za-z]{3}", k), f"bad index {k}"
    assert all(a < b for a, b in zip(idx, idx[1:])), "indices not strictly ascending"
    rects = [e for e in els if e["type"] == "rectangle"]
    for e in els:
        if e["type"] == "text" and e.get("containerId"):
            c = by[e["containerId"]]
            assert {"type": "text", "id": e["id"]} in (c["boundElements"] or []), "container missing text"
        for be in e.get("boundElements") or []:
            other = by[be["id"]]
            if be["type"] == "text":
                assert other["containerId"] == e["id"], "text containerId mismatch"
            else:
                ids = (other["startBinding"]["elementId"], other["endBinding"]["elementId"])
                assert e["id"] in ids, "boundElements lists an arrow not bound to it"
    arrows = [e for e in els if e["type"] == "arrow"]
    for a in arrows:
        for end, pt in (("startBinding", a["points"][0]), ("endBinding", a["points"][-1])):
            bnd = a[end]
            assert bnd and bnd.get("elementId") in by, f"{a['id']} {end} unbound"
            t = by[bnd["elementId"]]
            assert {"type": "arrow", "id": a["id"]} in (t["boundElements"] or []), "target missing arrow"
            fx, fy = bnd["fixedPoint"]
            ex, ey = t["x"] + t["width"] * fx, t["y"] + t["height"] * fy
            assert abs(a["x"] + pt[0] - ex) < 0.5 and abs(a["y"] + pt[1] - ey) < 0.5, \
                f"{a['id']} {end} endpoint not on its fixedPoint"
            assert bnd["mode"] in ("orbit", "inside")
        pts = [(a["x"] + p[0], a["y"] + p[1]) for p in a["points"]]
        ends = {a["startBinding"]["elementId"], a["endBinding"]["elementId"]}
        for p, q in zip(pts, pts[1:]):
            assert abs(p[0] - q[0]) < 0.01 or abs(p[1] - q[1]) < 0.01, f"{a['id']} diagonal segment"
            for r in rects:
                if r["id"] not in ends:
                    assert not _seg_hits_rect(p, q, r), f"{a['id']} crosses box {r['id']}"
    frames = {e["id"]: i for i, e in enumerate(els) if e["type"] == "frame"}
    for i, e in enumerate(els):
        if e.get("frameId"):
            assert e["frameId"] in frames, "frameId points at no frame"
            assert i < frames[e["frameId"]], "frame child after its frame"
            f = by[e["frameId"]]
            if e["type"] != "arrow":
                assert f["x"] <= e["x"] and e["x"] + e["width"] <= f["x"] + f["width"] + 0.5 and \
                       f["y"] <= e["y"] and e["y"] + e["height"] <= f["y"] + f["height"] + 0.5, \
                    f"{e['id']} outside its frame"
            if e["type"] == "text" and e.get("containerId"):
                assert by[e["containerId"]]["frameId"] == e["frameId"], "text and container in different frames"
    for fid in frames:
        fr = by[fid]
        for gid in frames:
            if gid != fid:
                g = by[gid]
                assert fr["x"] + fr["width"] <= g["x"] or g["x"] + g["width"] <= fr["x"] or \
                       fr["y"] + fr["height"] <= g["y"] or g["y"] + g["height"] <= fr["y"], "frames overlap"
    for rid, raw in notes_in:
        txt = next(e for e in els if e.get("containerId") == rid)
        assert txt["originalText"] == raw, "note reworded"
        assert txt["text"].replace("\n", "").replace(" ", "") == raw.replace("\n", "").replace(" ", ""), \
            "note display text lost characters"
    return {"elements": len(els), "arrows": len(arrows), "frames": len(frames)}


def selftest():
    spec = {"title": "selftest", "foot": "foot",
            "frames": [
                {"name": "A", "status": "open", "hub": "Hub A",
                 "have": ["e1", ["e2 linked", "https://example.com"], "!private e3", "e4 " * 30],
                 "shows": ["p1"], "calls": ["c1", "c2"],
                 "notes": ["Resaerch\n- typo kept  verbatim\n perhaps"]},
                {"name": "B", "hub": "Only hub"},
                {"name": "C", "hub": "Wide", "calls": ["c"] * 2, "notes": ["n1", "n2", "n3"]},
                {"name": "D", "hub": "D", "shows": ["x"] * 8}],
            "columns": 3}
    scene, B = build(spec)
    info = validate(scene, B.notes_in)
    assert info["frames"] == 4 and info["arrows"] == 4 + 1 + 3 + 5 + 8, info
    assert any("split the frame" in w for w in B.warnings)
    link = [e for e in scene["elements"] if e.get("link")]
    assert len(link) == 1 and link[0]["link"] == "https://example.com"
    assert any(e["type"] == "rectangle" and e["backgroundColor"] == COLORS["priv"] and
               e["strokeStyle"] == "dotted" for e in scene["elements"])
    assert frac_index(61) < frac_index(62) < frac_index(62 + 3843) < frac_index(62 + 3844)
    ex = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "examples", "spec.example.json")
    if os.path.exists(ex):
        s2, B2 = build(json.load(open(ex)))
        print("example:", validate(s2, B2.notes_in))
    print("selftest ok:", info)


if __name__ == "__main__":
    if "--selftest" in sys.argv:
        selftest()
        sys.exit(0)
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    scene, B = build(json.load(open(sys.argv[1])))
    info = validate(scene, B.notes_in)
    json.dump(scene, open(sys.argv[2], "w"), ensure_ascii=False)
    for w in B.warnings:
        print("warning:", w, file=sys.stderr)
    print(info)
