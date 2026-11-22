---
name: whiteboard-map
description: Turn a messy pile of information (a plan, an inventory, content decisions, options, open questions) into a clear Excalidraw+ whiteboard. One named frame per subject, a mind map inside each, colour plus a second cue per kind, arrows bound to boxes. Use when the user says "put it on the board", "map this out", "whiteboard it", "I can't see the shape of this", asks to plan, map, organise or visualise information, or mentions Excalidraw. Not for flowcharts of a strict process, or for long prose (that stays in a doc, such as a Notion page, and the board links to it).
---

# whiteboard-map

The user grounds plans visually. A board is a map they can switch around in: one
subject at a time, each in its own space. The board holds boxes, arrows and links;
long text stays in a doc (Notion or similar) and boxes link to it.

## When to use

- A plan or inventory has grown past what reads well in chat.
- The user asks for a board, a map, or says they can't see the shape of something.
- An existing board needs rebuilding after new decisions.

Don't use it for one-off answers, or for a strict sequence where order is the
point (draw a small flowchart instead).

## Workflow

1. **Gather.** List every item: facts or evidence, where things show up, open
   decisions, the user's own notes. Keep the user's notes as exact strings.
2. **Group by subject.** Cluster items by what they are about (Research, Education,
   Professional work...), never by process step. Each subject is one frame.
3. **Choose frames per concern.** Aim for 4–9 frames on a board. Split a frame when
   one side passes 7 items; split the board past 9 frames.
4. **Mind map inside each frame.** Hub (the subject) in the centre. Evidence on the
   left, platforms or destinations on the right, open calls then the user's notes in
   one row underneath. One box per item: more boxes, not fewer words.
5. **Status goes on the frame name**, e.g. `Research · evidence ✓ · build later`
   (spec field `status`, or write it into `name`). It is a label, never the layout.
6. **Legend at the top**, left to right, same order as the kinds below.
7. **Build and validate**: run the script (below). It refuses to write a scene
   with unbound arrows, crossings, diagonal segments, or reworded notes.
8. **Render and look**: `excalidraw render out.excalidraw --out out.png`, then open
   the PNG. Check one frame up close with `--frame-id <id> --scale 2`.
9. **Read the live board before overwriting.** `excalidraw scenes content get <id>
   --out live.json`. Diff against your spec; any text the user typed on the board
   goes into `notes` word for word (typos included).
10. **Write**: new scene with `scenes create --file`, or `scenes content put` on an
    existing one only after step 9 and the user's go-ahead. Back up `live.json`.

## Layout rules (numbers used by `scripts/build_board.py`)

| Rule | Number | Why |
|---|---|---|
| Space between frames | 180 px (about 10x the in-group gap) | Proximity: whitespace separates unrelated groups; common region (the frame) is the strongest grouping cue. |
| Gap between boxes in a column | 18 px | Proximity: tight spacing reads as one group. |
| Gap side column ↔ hub | 80 px, elbow trunk at its midpoint | Room for orthogonal arrows without touching boxes. |
| Gap side columns ↔ bottom row | 60 px, arrow bus at its midpoint | Bottom-row arrows run in clear space, never over boxes. |
| Child box width | 300 px; hub 220 px, 20 pt, bold border | Uniform size = same importance (similarity); hub differs in scale and border (visual hierarchy, max 3 sizes: title 36, hub 20, box 16). |
| Groups per frame | 4 (evidence, platforms, calls, notes) | Working memory holds about 4 chunks (Cowan); the frame is read as 4 chunks, not N boxes. |
| Items per side | warn above 7 | Upper end of chunk span; past it, split the frame. |
| Frames per board | warn above 9; 3 per row | 3 rows of 3 keeps each row a chunk; Novak suggests 15–25 concepts per map. |
| Frame width | at least 1060 px; widens so the bottom row is one row | Bottom row never wraps, so no arrow has to pass a box. |
| Reading order | title + legend top-left, frames left→right, top→bottom; most important frame first | F/Z scanning starts top-left. |

**Arrows.**
- Every arrow is bound at both ends: `startBinding`/`endBinding` carry `elementId`,
  `fixedPoint` and `mode: "orbit"`. Both boxes list the arrow in `boundElements`.
  Then the arrow follows a box when it is dragged. No free-floating arrows.
- Arrows are elbowed (`elbowed: true`): only horizontal and vertical segments, so
  none fan out at steep angles. Side children get hub edge → trunk → child edge.
  Bottom children get hub bottom → bus line → child top. A child level with the
  hub gets a straight line. The rule: **no diagonal segments, ever; 0 crossings**.
- Side columns are vertically centred on the hub, so most elbows stay short.
- Edge crossings hurt graph comprehension most (Purchase). The script asserts
  that no arrow segment passes through a box other than its two ends.

**Ink.** No decoration, shadows or extra lines. Every mark is a box, an arrow, a
frame or a label (Tufte's data-ink idea). Arrows are mid-grey (#868e96) so boxes lead.

## Colour legend (always plus a second cue)

| Kind | Fill | Second cue | Spec field |
|---|---|---|---|
| Evidence, what exists | blue #a5d8ff | left of hub | `have` |
| Platform, where it shows | purple #d0bfff | right of hub | `shows` |
| Open call, a decision | orange #ffd8a8 | bottom row, first | `calls` |
| The user's note, verbatim | yellow #fff3bf | bottom row, last, **dashed** border | `notes` |
| Private, not claimable | red #ffc9c9 | **dotted** border | leading `!` on an item |
| Hub, the subject | white | centre, larger text, thick border | `hub` |

These pastels sit close in lightness, so colour-blind viewers may not tell them
apart. Position and border style carry the kind as well (WCAG 1.4.1: colour is
never the only cue). Don't add text tags like "[evidence]" to boxes; that costs
words, and position already does the job.

## Anti-patterns

- **Process columns** (evidence → claims → content → calls) with dense text
  blocks. Rejected by the user: "very data dense", "too samey". Process is a label.
- One big block of text per subject. Split it into one box per item.
- Free-floating arrows (`startBinding: null`), or arrows drawn diagonally across
  the frame from a hub at a fixed position.
- Arrows to a wrapped second bottom row that cut through the first row.
- Colour as the only cue for a kind.
- Rewording, fixing typos in, or summarising the user's notes.
- Overwriting a live board without reading it first, or editing a scene you were
  not asked to touch.
- Copying long text onto the board. Link to the doc instead (`["text", "url"]`).

## Running the script

```bash
# From this skill folder:
python3 scripts/build_board.py --selftest
python3 scripts/build_board.py examples/spec.example.json /path/out.excalidraw
excalidraw render /path/out.excalidraw --out /path/out.png    # needs sandbox off
excalidraw scenes create --name "<name>" --collection-id <collectionId> --file /path/out.excalidraw
```

- The `excalidraw` CLI must run outside the Bash sandbox (DNS for the API and the
  local browser used by `render` both fail inside it).
- The CLI says personal keys may use `--collection-id private`, but on 2026-11-08
  it returned 404 "Collection not found" with our key. Pass a real collection id
  the user names.
- Warnings (too many items per side, too many frames) go to stderr; act on them
  by splitting, not by shrinking text.

**Spec format** (full example in `examples/spec.example.json`):

```json
{"title": "Board title", "foot": "optional footer",
 "frames": [{"name": "Research", "status": "evidence ✓",
             "hub": "Research",
             "have": ["6 papers", ["Scholar profile", "https://..."]],
             "shows": ["LinkedIn: Publications", "!Never public: X"],
             "calls": ["Which papers first?"],
             "notes": ["the user's exact words\n- typos kept"]}],
 "columns": 3}
```

Use `"rows": [[frame, ...], ...]` instead of `frames` + `columns` to fix rows by hand.

## What the selftest asserts

Unique ids; fractional `index` keys valid and strictly ascending; every arrow bound
at both ends to an existing element that lists it back, endpoint exactly on its
`fixedPoint`; only orthogonal segments; no segment crosses a non-endpoint box;
frame children come before their frame and sit inside it; frames don't overlap;
bound text and container agree; notes keep `originalText` identical to input.
The same validator runs on every build.

Sources for each rule: `references.md`.
