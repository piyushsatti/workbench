---
name: notion-page-design
description: Build or reorganise Notion pages, hubs and workspaces so a human can review and navigate them at a glance. Use when building a Notion hub, a project database, a review or approval queue, a drafts or proposals tracker, a page template, or when the user says "make this Notion page readable", "clean up this Notion page", "organise my Notion", "set up a review queue", or "where do proposals go". Covers hub page vs full-page database vs inline database vs linked view, a Status-based review workflow, scannability limits, and Notion MCP specifics.
---

# notion-page-design

The user reviews proposals, drafts and images in Notion, rewords them, and approves
them before posting. Pages must show what needs the user first, keep one source of
truth, and stay shallow.

## When to use

- A new hub, project database, review queue or page template is needed.
- A Notion page has grown into a dump that's hard to scan.
- Drafts or proposals need a place to wait for review.

Not for long-form writing itself (just write it), or for visual maps (use whiteboard-map).

## Workflow

1. **Read first.** `notion-fetch` the page or database, and its children, before
   proposing anything. Note the existing pages, databases, properties and views, and
   any text the user wrote.
2. **Never delete, move, rename or unsync the user's pages without asking.** Propose
   the move; the user approves each one. Prefer archiving over deleting.
3. **Draft the structure as a tree** (see `examples/structure.example.md`): pages,
   databases, properties, views, templates. Show it to the user and get a yes.
4. **Build** in this order: databases and properties → relations → templates →
   views → hub page blocks. Fetch again after each step and check the shape.
5. **Check the result** as the user would see it: the top of the hub fits on one
   screen, the `Needs you` view shows only what needs action, and links resolve.

## Information architecture rules

| Use | When | Notes |
|---|---|---|
| Hub page | The front door to an area | Needs-you block, short links or views, a "how this works" toggle |
| Full-page database | The database is the page's main content | Sits in the sidebar; lock its structure once settled |
| Inline database | A small table that belongs inside one page's context | Its controls hide until hover |
| Linked view | The same rows shown elsewhere, filtered | One source of truth; filters apply only to that view |

- **One database per kind of thing.** All proposals live in one Proposals database
  related to Projects. Each project page shows a linked view filtered to itself. A
  per-project child database is the alternative; it loses the cross-project queue, so
  ask before choosing it.
- **Depth: 3 levels normally, 4 at most** (hub → database → item → rare sub-page).
  Breadcrumbs help on deep pages, but a shallow tree needs fewer of them.
- **One canonical parent per page.** Show it elsewhere through linked views and
  mentions, never through copies.
- **Status lives in a property**, not in the title or a page heading.

## Review-status workflow

Use one Status property, with options in Notion's three groups:

- To-do: `Draft`
- In progress: `Ready for review`, `Changes requested`
- Complete: `Approved`, `Posted`

The loop:
1. The agent writes the draft from the template (Draft) and sets Ready for review.
2. The user rewords in place, or turns on Suggest edits so changes are visible, and
   leaves inline comments.
3. The user sets Approved, or Changes requested with a comment.
4. After posting: Posted, with the Posted URL and date filled in.

The agent never sets Approved or Posted by itself. Those are the user's actions.

Standard views on the Proposals database:
- `Needs you`: list, filter Status = Ready for review, sort by Target date.
- `Pipeline`: board grouped by Status, empty groups hidden.
- `Images`: gallery with Media as the cover.
- `Posted`: table, newest first.

Version history is kept for 7 days on Free, 30 on Plus and 90 on Business. Keep the
approved copy on the page itself; don't rely on history.

## Scannability rules (with limits)

- **Action first** (inverted pyramid): a page opens with its decision or summary.
- **Headings**: H1 = the page title only; H2 for sections; H3 rarely. At most 3 text
  sizes. Headings say the point ("Final copy to approve"), not a label ("Section 2").
- **Toggles for detail**: context, sources and earlier versions go in toggles. One
  level of toggles; never nest disclosure past 2 levels.
- **Callouts**: at most 1 per page, for the decision. More callouts weaken the cue.
- **Columns**: at most 2. The primary item goes in the left column, because columns
  stack left-then-right on phones. Use them to put an image beside its copy.
- **One idea per paragraph; bullets over prose; about half the words** (Nielsen:
  16% of readers read word by word).
- **Views**: about 4–7 items visible per group. Filter queues to actionable items
  only; group long views by Status or Project instead of scrolling a flat list.
- **The review page fits about one screen** above the first toggle: decision callout,
  final copy, image. (A heuristic.)
- **Colour**: give each Status the same colour everywhere; reserve one colour
  (e.g. yellow) for "needs you". Status text carries the meaning too, never colour alone.

## The "Needs you" block

At the top of every hub, before anything else:

1. A callout with one line: what's waiting, and what "approve" means here.
2. Directly under it, a linked view of the work database: list layout, filter
   Status = Ready for review (or Changes requested, assigned to the agent), showing
   only Name, Project, Platform and Target date.
3. When the view is empty, the callout says so ("Nothing waiting"). Never pad it.

## Anti-patterns

- **Wrapper pages** that only hold a single database. Make the database full-page instead.
- **Duplicate project lists**: a second database of the same projects. Exception:
  the user wants hub sections such as "Active Projects" hand-curated, with one
  heading and one link per project. Never replace those with a live gallery or list
  view; that was tried and rejected (2026-11-08) as ugly and confusing.
- **Flat page dumps**: dozens of sibling pages with no database, no properties and no
  views. Move them into a database with Status and Type, after asking.
- Status written into titles ("[DRAFT] Post about X").
- One database per project for the same kind of item when a cross-project queue is needed.
- Deep nesting: sub-pages of sub-pages, toggles inside toggles.
- Many callouts or colours on one page; colour as the only cue.
- Synced blocks with more than 10 copies: deleting the original or "Unsync all"
  removes every copy, and Undo won't help. Use a linked view or a mention instead.
- Pre-filled relations in database templates (every new page links to the same page).

## Notion MCP specifics

- **Fetch before editing**: `notion-fetch` the page, database or `collection://` data
  source first. Use the exact property names and the data source ID it returns.
  Check `truncated` and `unknown_block_ids` before trusting the content.
- **Markdown syntax**: read `notion://docs/notion-flavored-markdown-spec` (pass it to
  `notion-fetch`) before writing page content. Don't guess callout, toggle or column syntax.
- **Filenames**: wrap every filename (`notes.md`, `SKILL.md`) in inline code. Notion
  autolinks bare `.md` names because `.md` is a real domain. After replacing a table,
  re-fetch and check that no empty column was added.
- **Create a database**: `notion-create-database` with `parent: {page_id}` and a
  `schema` in SQL DDL, for example:
  ```sql
  CREATE TABLE ("Name" TITLE,
    "Status" STATUS('Draft':gray:to_do, 'Ready for review':yellow:in_progress,
                    'Changes requested':orange:in_progress,
                    'Approved':green:complete, 'Posted':blue:complete),
    "Type" SELECT('Post':blue, 'Profile edit':purple, 'Image':pink, 'Page copy':gray),
    "Platform" MULTI_SELECT('LinkedIn':blue, 'X':default, 'Website':green, 'GitHub':gray),
    "Media" FILES, "Target date" DATE, "Posted URL" URL,
    "Project" RELATION('<projects data source id>', DUAL 'Proposals'),
    "Last edited" LAST_EDITED_TIME)
  ```
  The result returns the new data source ID in a `<data-source>` tag.
- **Create views**: `notion-create-view` with `data_source_id` plus either
  `database_id` (adds a view tab) or `parent_page_id` (appends an inline linked view to
  a page, like `/linked`). Read `notion://docs/view-dsl-spec` before writing `configure`.
  Examples: `FILTER "Status" = "Ready for review"; SORT BY "Target date" ASC` and
  `GROUP BY "Status" BY GROUP HIDE EMPTY` (BY GROUP applies to Status properties only).
- **Create rows**: `notion-create-pages` with `parent: {data_source_id}`. Fetch the
  database first. Relations take page URLs or IDs; dates split into `date:<prop>:start`.
- **Placement**: `parent_page_id` views append at the end of the page. Create the hub's
  `Needs you` view first, or move it to the top in the app.
- **Stay in scope**: build only what the approved tree lists. Report any tool warnings.

Sources: `references.md`.
