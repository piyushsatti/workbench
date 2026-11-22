# References for whiteboard-map

Fetched 2026-11-08 unless marked (book) or (not fetched).

## Grouping (Gestalt)
- NN/g, Proximity Principle in Visual Design: https://www.nngroup.com/articles/gestalt-proximity/ — related items close, unrelated items apart; whitespace alone forms groups. Gives no pixel numbers, so the 18 px / 180 px gaps are our choice.
- NN/g, The Principle of Common Region: https://www.nngroup.com/articles/common-region/ — a boundary "can overpower" proximity and similarity; use frames as the region; avoid extra borders that add clutter.
- NN/g, Similarity Principle in Visual Design: https://www.nngroup.com/articles/gestalt-similarity/ — same colour, shape or size reads as related; apply one treatment per kind, consistently.
- NN/g, Visual Hierarchy in UX: https://www.nngroup.com/articles/visual-hierarchy-ux-definition/ — scale, contrast, grouping; at most 3 sizes; make the most important element biggest. Basis for title 36 / hub 20 / box 16.

## Cognitive load and chunking
- Cowan (2001), The magical number 4 in short-term memory, Behavioral and Brain Sciences 24(1): https://pubmed.ncbi.nlm.nih.gov/11515286/ — capacity about 4 chunks (3–5). Basis for 4 groups per frame and the 3-per-row frame grid.
- Miller (1956), The magical number seven, plus or minus two: https://psychclassics.yorku.ca/Miller/ (not fetched) — the older 7±2 estimate, which Cowan calls a rough rhetorical figure. Used only as the upper warning bound (7 items per side).
- Novak and Cañas, The Theory Underlying Concept Maps: https://cmap.ihmc.us/docs/theory-of-concept-maps — focus question per map; 15–25 concepts usually suffice; avoid "string maps". Basis for frames-per-board ceiling and one focus (hub) per frame.

## Reading order
- NN/g, F-Shaped Pattern of Reading on the Web: https://www.nngroup.com/articles/f-shaped-pattern-reading-web-content/ — people scan top-left first, then down the left; front-load key content. Basis for title and legend at top-left and frames ordered by importance.

## Graph readability
- Purchase (1997), Which aesthetic has the greatest effect on human understanding?, Graph Drawing (LNCS 1353): https://doi.org/10.1007/3-540-63938-1_67 (not fetched; paywalled) — edge crossings matter most, then bends. Basis for the 0-crossings assertion. Elbows add bends; we accept that to remove steep diagonals the user objected to.

## Ink and mind maps
- Tufte, The Visual Display of Quantitative Information (book, 1983/2001) — data-ink: remove non-data marks. The site mentions the data-ink ratio in its sparklines notes (https://www.edwardtufte.com/notes-sketches/). Basis for no decoration and grey arrows.
- Buzan, The Mind Map Book (book, 1993) (not fetched) — central topic with radiating branches, one idea per branch. Basis for hub in the centre and one box per item.

## Colour
- W3C, Understanding SC 1.4.1 Use of Color: https://www.w3.org/WAI/WCAG22/Understanding/use-of-color.html — colour must not be the only visual means of telling things apart; add text, pattern or shape. Basis for position + border-style cues.
- Okabe and Ito, Color Universal Design: https://jfly.uni-koeln.de/color/ — combine colour with "shapes, positions, line types"; label on the figure rather than relying on a key. Basis for dashed (notes) and dotted (private) borders.

## Excalidraw specifics
- Excalidraw element types (master): https://raw.githubusercontent.com/excalidraw/excalidraw/master/packages/element/src/types.ts — `FixedPointBinding = {elementId, fixedPoint, mode}`, `mode` is "inside" | "orbit" | "skip"; `focus`/`gap` no longer in the type; `ExcalidrawArrowElement.elbowed`; `BoundElement {id, type: "arrow"|"text"}`; `frameId` on every element.
- Excalidraw restore.ts (master): https://raw.githubusercontent.com/excalidraw/excalidraw/master/packages/excalidraw/data/restore.ts — legacy `{focus, gap}` bindings migrate to `{fixedPoint, mode}`; stale bindings are nulled; `boundElements` are repaired; invalid fractional indices are re-synced. We write both forms so old and new readers bind.
- Excalidraw docs, Frames: https://plus.excalidraw.com/docs/sdk/codebase/frames — frame children must come before the frame element in the array, or clipping breaks.
- Excalidraw docs, JSON schema: https://plus.excalidraw.com/docs/sdk/codebase/json-schema — top-level `type`, `version`, `source`, `elements`, `appState`, `files`.
- Local memory note on the CLI (2026-10-23): every element needs a fractional `index`; run outside the sandbox; `scenes content get` before any put; `scenes render` for a visual check.
