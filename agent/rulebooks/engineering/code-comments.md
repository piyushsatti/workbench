# Rulebook template: code comments and documentation

Portable template. A project deriving from this keeps the rules as written, replaces the illustrative examples with its own, and fills in the Project binding section at the end.

Principle: code documents itself, and documentation effort concentrates at boundaries.

## Comments
- Code explains itself through names, structure, and types. Comments are the fallback.
- A comment states only what the code cannot: rationale, non-obvious invariants, external constraints. Never what the next line does.
- Small, precise, only as needed. Long rationale goes in a design doc that explains the choice and its reasoning; the comment stays short.
- No ticket numbers, spec sections, or decision and ruling ids (such as ADR-12) in a comment, a docstring, or a document that ships with the repository, in any language. The reason lives in the text itself; ticket ids belong in commit messages and PR descriptions.
- A path written in code, a comment, a docstring, a config example or a document that ships with the repository must resolve inside the repository, written relative to its root. Never an absolute path from a developer's machine, and never a path into the workspace around the repository (a notes, artifacts or worktrees directory a fresh clone does not contain). When the reason lives only outside the repository, state the reason in place. A source file is cited by its full path from the repository root (`src/<package>/module.py`, not `module.py`). A path on a running machine, such as `/etc/...` or `/mnt/...`, is a runtime fact and is written as it is.

## Python: docstrings and the boundary rule
Every public item gets a docstring, in the numpy convention, enforced by the linter's full `D` rule set.

- Every public module, class, method and function has a docstring. Public means the name has no leading underscore.
- A docstring is one summary line ending with a period, and nothing more unless the signature cannot say what the item is for. The summary of a function or method is in the imperative mood ("Return the user with this id."); the summary of a module, package, class or property is a noun phrase naming what it is ("Domain errors raised by the naming helpers.", "The directory this source reads."); an exception class may instead state the condition it signals. The linter enforces the shape of function summaries only.
- A function that raises adds a `Raises` section: the heading underlined with dashes, each exception name on its own line, its condition indented below. This is the declared error contract.
- No `Parameters` or `Returns` sections. The signature carries parameter and return types, so those sections restate it.
- Backticks follow numpydoc in all Python. Parameter and Python object names take single backticks. Code, SQL, paths, and table or column names take double backticks. `#` comments are exempt.
- Interface functions, meaning the seams between different kinds of stuff such as API endpoints, protocol handlers, the database access layer, and anything one component calls on another, also validate at runtime what crosses the boundary.
- Private functions need no docstring and no runtime validation. They trust their callers.
- All functions, public and private alike, declare parameter types, a return type, and their error contract.

Example:

```python
def fetch_user(user_id: UUID) -> User:
    """Return the user with this id.

    Raises
    ------
    UserNotFoundError
        No user has this id.
    """
```

## TypeScript
- Being exported is not a reason to carry a doc comment. Names, signatures, types, and the file and folder structure carry the what; a comment is warranted only when it adds something none of those can.
- Write a comment for exactly three things: a non-obvious trap, the meaning of a unit or of a null or undefined value, or a cross-file invariant that a reader of this file alone would not know.
- These are defects, and a reviewer reports them as such:
  - restating a name, a signature, or a field,
  - naming a consumer or a producer, which a symbol search answers for free and which goes stale on the next rename,
  - inventorying usages or listing call sites.
- When an edit makes a comment redundant, delete it in that edit. A deleted comment is intended, not a regression a reviewer should flag.

## Everything else
- Python module docstrings follow the Python section above. In TypeScript, a module comment only where a module's purpose is not obvious from its name and its exports.
- Architecture-level why goes to the project's architecture documentation, per the architecture-documentation rulebook, and never into code comments.

## Project binding
A deriving project states:
- which validation library enforces the boundary rule,
- its own illustrative examples of a warranted comment,
- any language beyond Python and TypeScript that it uses, and the equivalent boundary rule for it.
