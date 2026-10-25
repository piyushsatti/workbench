# Rulebook template: error handling

Portable template. A project deriving from this keeps the rules as written, replaces the illustrative examples with its own, and fills in the Project binding section at the end.

Errors use exceptions plus a declared contract, in preference to Result or Either types as a general mechanism.

## The mechanism
- Errors travel as exceptions.
- One domain exception hierarchy rooted at a single project error type. Domain code raises subclasses; catching code targets a level of the hierarchy, never a raise site.
- Expected domain outcomes are not exceptions. A check that finds a mismatch, or a scan that finds nothing, returns a value or an enum. An exception means something actually went wrong.
- Exceptions carry structured context as attributes, not just a message string.

## The contract
- Every function declares its parameter and return types.
- Public functions carry a numpy-style docstring with a `Raises` section listing each raisable exception and the condition that raises it. This is the declared error contract.
- Private functions need no docstring; their raisable errors are implied by the hierarchy. Add a one-line raises note only when it is non-obvious.
- Never `except Exception: pass`. Catch what you can handle and let the rest propagate to a boundary.
- A bare `except:` with no exception type is forbidden on the same grounds.

## Boundaries translate, never leak
- HTTP APIs translate domain exceptions into RFC 9457 `application/problem+json` responses carrying type, title, status, detail, and instance. One catch-all handler covers unhandled exceptions: log with full traceback, return a generic problem response. Raw stack traces never cross the API.
- Between services, failures become domain events carrying a pointer to the diagnostic log.
- WebSockets wrap their receive loops and close with an explicit close code on a protocol or validation error. A socket never dies without a close frame.

## Validation
- Runtime data validation happens at boundaries only. Internal functions trust their callers, and types are the internal contract.

## Frontend
- API errors arrive as problem+json, and one client-side translation layer maps them to user-facing copy stating what happened, what it means, and what to do. Components never branch on raw status codes.

## Project binding
A deriving project states:
- the root of its exception hierarchy and the subclasses that exist,
- what its structured context attributes carry,
- any connection whose loss has defined semantics, and what those semantics are.
