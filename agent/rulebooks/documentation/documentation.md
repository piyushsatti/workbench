# Documentation

For maintained documents, the project's documentation standard governs minimum body contents, contextual links and the header. Preserve useful source text and collection-specific contracts.

- State structure, why it exists, and how to operate it. Remove preamble, filler, and repetition.
- Give long documents a short orientation and a useful reading route. Review reports may use verdict bullets and compact evidence tables; ordinary pages follow the reader's questions.
- For a pattern assessment, `Pattern | Verdict | One line` is a useful table. Explain or link terminology when the intended reader needs it.
- Use informative headings and focused paragraphs. Use lists for steps or parallel points, and tables when comparison is clearer than prose.
- Link supporting evidence or relevant documents where the claim or relationship appears in prose. Use a dedicated `file:line` or evidence column for comparable findings when useful; it does not replace contextual links.
- Use diagrams when they explain structure or behavior more clearly. Give each a clear purpose and split dense diagrams into understandable views.
- Prefer minimal and complete documentation. Detail needed only on request stays outside the page.

## Architecture pages

The following layout is for reviews comparing observed architecture with a requested target. Use it when that comparison is the document's purpose; it is not a mandatory outline for every source account, decision or explanation.

Separate observed current state from requested target design with a rule and a final `How <the thing> should work` section.

- Start target design with `# | Requirement | Because`. Each requirement cites a real finding from the observed section.
- Surface a requirement that contradicts an existing principle as a design fork with both branches. Do not resolve it silently.
- Give each structure section one diagram plus tables. Component tables include a `Must not` column.
- End with `What these requirements make impossible`, mapping findings to requirements, and `Still open at the requirement level`.
- Keep cost, effort, ordering, feasibility, and migration mechanics out of target requirements. Put migration in a plan.

Project-specific navigation and decision ledgers are project-owned; see the [orchestration procedure](../orchestration/procedures.md).

