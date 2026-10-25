# Rulebook template: logging

Portable template. A project deriving from this keeps the rules as written, replaces the illustrative examples with its own, and fills in the Project binding section at the end.

Two kinds of record exist and are never conflated: domain events, which are the durable audit record, queryable and operator-facing, and diagnostic logs, which are for engineers debugging. This rulebook governs the second.

## Library and shape
- Structured logging with JSON output, one configuration module per component. In Python, `structlog`.
- Bind context once, early, and let it thread. A single correlation id traces a request through every component that handles it.
- Logs go to stdout and stderr so the container runtime collects them. Long-running workers may additionally keep rotated files. Never to the database.
- No bare `print()` in production code paths.

## Levels
- DEBUG: development detail, off in production.
- INFO: business happenings worth a line.
- WARNING: a recoverable anomaly, no operator action needed yet.
- ERROR: an operation failed. Include full context and traceback.
- CRITICAL: reserved, and effectively unused.

## What not to log
- No secrets, tokens, or credentials, ever. A processor masking known keys is a backstop, not the rule.
- No full request or response bodies at INFO and above.
- No log-and-rethrow at every level. An error is logged once, where it is handled.

## Relationship to domain events
- If an operator should be able to see it in the interface or be notified of it, it is a domain event, not a log line. When a failure produces an event, the event carries a correlation id pointing into the diagnostic logs. The event says what happened; the log says why.

## Frontend
- One thin logger wrapper, and no bare `console.*` in components. Tag with the correlation id from response headers. Production builds log warnings and errors only, and never log user input.

## Project binding
A deriving project states:
- the names of its correlation ids and where each is bound,
- where its domain events are stored,
- its per-component logging configuration modules.
