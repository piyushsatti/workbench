# Example structure: Home hub

A tree to show the user before building anything. Names in backticks are views.

```
Home                                     hub page (pinned)
├─ callout  "Needs you: 3 items waiting. Approve = ready to post as written."
├─ linked view  Proposals › `Needs you`   list · Status = Ready for review · sort Target date
│                                          shows Name, Project, Platform, Target date
├─ H2 Projects
├─ Projects                              full-page database (child of Home)
│   │  props: Name · Status (Active/Paused/Done) · Area · Proposals (relation)
│   │         · Open reviews (rollup: count of Proposals where Status = Ready for review)
│   │  views: `All` table · `Active` gallery
│   ├─ Website relaunch                  row page, from the "Project" template
│   │   ├─ callout  goal in one line + current status
│   │   ├─ linked view  Proposals › `Website pipeline`  board by Status · Project = Website relaunch
│   │   ├─ H2 Context  (links to long docs)
│   │   └─ toggle  Decisions log
│   ├─ Newsletter
│   └─ Home lab
├─ Proposals                             full-page database (child of Home), the one source
│   │  props: Name · Status · Project (relation ↔ Projects.Proposals) · Type · Platform
│   │         · Media (files) · Target date · Posted URL · Last edited
│   │  views: `Needs you` list · `Pipeline` board · `Images` gallery · `Posted` table
│   └─ "LinkedIn catch-up post"          row page, from the "Proposal" template
│       ├─ callout  "Decision needed: approve the copy and image below."
│       ├─ H2 Final copy                 the exact text to post
│       ├─ columns  [image] | [alt text + caption]
│       ├─ toggle  Context
│       ├─ toggle  Earlier versions
│       └─ H2 Your notes                 kept word for word
└─ toggle  How this works                status meanings, one line each
```

Depth: Home → Proposals → item = 3 levels. The Website relaunch page shows its proposals through
a filtered view, so the review queue stays in one database.

Alternative, only if the user prefers it: a Proposals child database inside each project
page. Projects stay fully separate, but the hub's `Needs you` queue then needs one linked
view per project.
