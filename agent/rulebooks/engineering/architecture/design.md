# Design and architecture

- The human defines the product, boundary, and contract. The agent determines implementation after those decisions are explicit.
- Every plan states affected artifacts and scope, untouched areas, reversibility, and rollback limits.
- Prefer composition, explicit dependencies, shared modules, and a clean dependency graph.
- Do not let migration cost silently weaken target requirements. Put sequencing and feasibility in a separate plan.
- Surface any requirement that contradicts a documented principle; present both branches and leave the choice open.

Ask these questions for every system:

1. Where does it need a circuit breaker? Bound retries and reconnects with a ceiling and a reachable failure state. State what trips the breaker, or why none is needed.
2. How can an operator distinguish healthy silence from a dead watcher? Use a heartbeat, dead-man check, or independent watcher, and bind the monitor to a durable lifecycle.

Before a multi-part design, architecture, or research effort, read
`../../orchestration/procedures.md`. Multi-part design produces
project-owned roadmap navigation and a decision ledger before design discussion
starts. Prioritize both across groups and within each group.

