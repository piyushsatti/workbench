# Rulebook template: architecture documentation

Portable template. A project deriving from this keeps the rules as written, replaces the illustrative examples with its own, and fills in the Project binding section at the end.

- All architecture documentation is written to the Google design doc standard: context and scope, goals and non-goals, the actual design, alternatives considered, cross-cutting concerns. Trade-offs are the point of the document. Record why, not just what.
- Design docs are point-in-time: dated, and never silently rewritten. A superseded doc carries a banner pointing at its successor.
- One doc, one decision or one system slice. A doc that needs two summaries is two docs.
- Every doc states its status: draft, reviewed, implemented, or superseded.
- Diagrams carry a date and a source, whether a tool or a text form, so they can be regenerated.

## Scope
- These rules govern design documents and also the component pages under the project's `docs/` directory, which describe how a component works today. A component page is exempt from the status and point-in-time rules above, because it is present tense and updated with the code it describes. It is not exempt from the diagram, content-rigor, and one-page-one-subject rules.
- The architecture reviewer covers both kinds of document.

## Diagrams
- Every flow described in an architecture doc gets a Mermaid diagram. Prose alone is not enough for anything that moves through more than two components.
- Complexity cap of seven to nine components per diagram. Exceeding it means the diagram is decomposed into an overview plus zoomed sub-diagrams, never one crowded picture.
- Communication between different systems is shown TCP/IP style: each system drawn as its stack of layers, peer layers talking logically across, and the actual wire at the bottom. A single arrow labeled "talks to" is not a description of an interface.

## Content rigor
- Claims of fact are distinguished from proposals. Nothing is presented as certain that is not.
- Deliberately open questions stay open and flagged as such. A reviewer never lets a doc silently resolve them.

## Project binding
A deriving project states:
- where its architecture documentation lives and whether that location travels with the repository,
- the component stack its diagrams draw, so the layering rule has something concrete to apply to.
