# References for notion-page-design

Fetched 2026-11-08 unless marked.

## Notion official help
- Intro to databases: https://www.notion.com/help/intro-to-databases — full-page when the database is the page's focus, inline when nested in other content; locking; property display options.
- Views, filters, sorts and groups: https://www.notion.com/help/views-filters-and-sorts — view types; per-view settings; AND/OR filters up to 3 levels; group and sub-group, hide empty groups.
- Using linked databases: https://www.notion.com/help/guides/using-linked-databases — one source of truth; edits sync back; filters apply only to that linked instance.
- Database properties: https://www.notion.com/help/database-properties — Status has fixed To-do / In progress / Complete groups; Files & media; read-only Created by and Last edited.
- Database templates: https://www.notion.com/help/database-templates — default and repeating templates; don't pre-fill relations; templates belong to one database.
- Synced blocks: https://www.notion.com/help/synced-blocks — edit once, update everywhere; access needs the original page; the over-10-copies delete trap.
- Columns, headings and dividers: https://www.notion.com/help/columns-headings-and-dividers — columns stack on phones; H1–H3; toggle headings; automatic table of contents once a page has 2 or more headings.
- Comments, mentions and reminders: https://www.notion.com/help/comments-mentions-and-reminders — page and inline comments; resolve and reopen; mentions notify and create backlinks.
- Suggested edits: https://www.notion.com/help/suggested-edits — suggesting mode; accept or reject; text-type blocks only; needs Can comment; blocked on locked pages.
- Duplicate, delete and restore: https://www.notion.com/help/duplicate-delete-and-restore-content — version history retention: 7 / 30 / 90 days / unlimited by plan.
- Style and customize (callouts): https://www.notion.com/help/customize-and-style-your-content — callouts highlight a tip or warning and can hold headings (search summary, not fetched).
- Writing and editing basics (toggles): https://www.notion.com/help/writing-and-editing-basics — toggles have no default state (search summary, not fetched).
- Keyboard shortcuts: https://www.notion.com/help/keyboard-shortcuts — cmd/ctrl+alt+T expands or closes all toggles in a list (search summary, not fetched).

## HCI
- NN/g, Breadcrumbs: https://www.nngroup.com/articles/breadcrumbs/ — useful on deep pages; unnecessary at 1–2 levels; one canonical trail per page.
- NN/g, Progressive disclosure: https://www.nngroup.com/articles/progressive-disclosure/ — frequent items up front; more than 2 disclosure levels lowers usability.
- NN/g, Inverted pyramid: https://www.nngroup.com/articles/inverted-pyramid/ — conclusion first in every page, heading and paragraph. Basis for the Needs-you block.
- NN/g, How users read on the web: https://www.nngroup.com/articles/how-users-read-on-the-web/ — 16% read word by word; headings, bullets, one idea per paragraph, half the words.
- NN/g, Proximity: https://www.nngroup.com/articles/gestalt-proximity/ — related items close together; copy beside its image.
- NN/g, Common region: https://www.nngroup.com/articles/common-region/ — a boundary overpowers proximity; too many borders clutter. Basis for one callout per page.
- NN/g, Similarity: https://www.nngroup.com/articles/gestalt-similarity/ — consistent colour per meaning.
- NN/g, Visual hierarchy: https://www.nngroup.com/articles/visual-hierarchy-ux-definition/ — at most 3 sizes; most important element most prominent.
- Cowan (2001), The magical number 4: https://pubmed.ncbi.nlm.nih.gov/11515286/ — about 4 chunks. Basis for short, grouped views.

## Tooling
- Notion MCP tool descriptions (`notion-create-database`, `notion-create-view`, `notion-create-pages`, `notion-fetch`), read in-session 2026-11-08 — SQL DDL schema with STATUS groups and DUAL relations; view DSL (FILTER, SORT BY, GROUP BY ... BY GROUP); `database_id` vs `parent_page_id` view placement.
- Local memory note on Notion autolinking (2026-10-25) — wrap `.md` filenames in inline code; check table shape after a replace.
