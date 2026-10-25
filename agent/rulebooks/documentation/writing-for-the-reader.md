# Rulebook: writing for the reader

Load when a document, ticket, PR, runbook or announcement is about to be written.

Pick the reader before writing. Every artifact has ONE primary reader, one question they are asking, and a style contract that answers it.

| Artifact | Primary reader | Their question | Style contract |
|---|---|---|---|
| Design page | Mixed audience, future readers, non-experts | "What's the problem and why does it matter?" | Incident/problem first in plain words; one-paragraph summary up top; jargon defined at first use or cut; "what we tested / what we found" tables; deep tech detail linked out, never inline |
| Issue tracker story/task | The implementer | "What do I build, how do I know it's done?" | Precise and technical is CORRECT here: exact paths, flags, configs; acceptance criteria; no narrative padding |
| Issue tracker bug | Triager | "How do I reproduce it, how bad is it?" | Repro steps, expected vs actual, evidence, blast radius |
| PR description | Reviewer | "Is this safe to merge?" | Terse and high level: what changed and why it is safe, a usage example, how verified. No file-by-file walk |
| Runbook / ops note | Operator mid-incident, stressed | "What do I do right now?" | Action first, numbered steps, pasteable command blocks, background at the bottom or not at all |
| Chat announcement | Skimmers | "Does this affect me?" | One line of what plus impact, link for everything else |
| Handoff / work-order doc | Future maintainer / future agent session | "What's the full state?" | Dense and precise is fine; completeness beats accessibility |

## Meta-rules

- **One artifact, one reader.** Two audiences means two artifacts, cross-linked. Never average the styles.
- **Jargon test.** Terms naming tool internals appear ONLY in implementer-facing artifacts. Elsewhere, say what the thing does.
- **Roles, not names.** Write "human" instead of a person's proper noun, and "agent" for any autonomous agent (a Claude session, subagent, bot or workflow step) instead of its product or instance name. A rule then still reads true when the person or the tool changes.
- **Say which environment and which version.** Whenever something is said to work, to be tested, to be running or to be checked, name the environment it is in (for example dev or staging) and the version it is at (a commit, a build or a release, in the project's own naming). Use the project's own words for both, never made-up labels such as "Part A" and "Part B". Each project defines its environments and its versioning scheme in its own docs, and its CLAUDE.md says where. Read them there rather than assuming them, and if a project defines neither, say so and ask.

## Register by medium

- **Chat replies: concise, accuracy and simplicity over grammar.** Say it in as few words as correctly convey it. Dropped words, fragments and imperfect grammar are fine if the meaning stays accurate. Tables still carry comparative data.
- **Explain in plain words: one idea per sentence, effect before mechanism, a library term explained once.** Dense sentences get rewritten, not defended. State what happens in practice first, then how it works.
- **Lead with the direct answer, then the detail.** When the user asks whether something works, open with the plain verdict in the terms the question was asked in ("Yes, once the config flag is set. But no job has actually run yet."), then a short numbered walkthrough from the operator's point of view, marking each step as watched working or only inferred. A table of verified components is evidence for an answer, not the answer, and leaves the user to reassemble it. Keep ready and done apart: every leg proven with no job run is ready. Findings, ownership and follow-ups come after the answer, never instead of it. When the reply is guidance, lead with the single next action, give one step at a time, keep the result to one or two lines, skip recaps of finished work, and put detail behind "want the details?", because long multi-section replies bury the one thing the user must do next.
- **"What do I do" or "what do I check now" gets a short numbered list of actions and nothing else.** No explanation unless asked.
- **HTML explainer and review pages are minimal.** A review page leads with the answer (what can be filled, what was found), puts detail behind clickable cards instead of dense tables, and puts images on the page the reader opens, not a separate page.
- **Issue tracker tickets: technical, specification English.** Exact paths, flags, configs, acceptance criteria. Precision over readability, as the table above already says.
- **A ticket states up front what is wrong or missing and why it matters**, in plain words, before the specification detail.
- **Every outward-facing action confirms first** with an explicit yes, however it was written: publishing, filing, and PR-visible actions (create, comment, description edits).

## Formatting

- **Never hard-wrap text.** In documents, documentation, agent and skill files, and code, one paragraph, list item or statement is one line however long. The formatter or the renderer handles width. Wrapped prose splits protocol lines and list items, and confuses grep and diff.
- **Prefer markdown tables over bullet lists** for options, tradeoffs and comparisons.
- **Long output goes to a file.** Write long runbooks and reference output to a markdown file, not just chat.
- **When the user asks to see something, put it in the chat reply, and give full absolute paths.** A file alone does not answer "show me"; the file is the extra copy. Show a changed JSON file as a diff. Show a rename as the current name beside the proposed one.
- **Neutral documentation, no blame.** In any persistent doc, state gaps and risks as plain facts, but never write blame, fault-implication or alarm about individuals or their practices. Keep the fact and the gap; drop the accusatory tone. Applies to every doc, not just published ones.
