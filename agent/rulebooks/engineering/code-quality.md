# Rulebook template: code quality, or what makes a pull request acceptable to review

Portable template. A project deriving from this keeps the rules as written, replaces the illustrative examples with its own, and fills in the Project binding section at the end.

Governs build, simplify and review of a pull request, alongside the code-comments, error-handling, and architecture-documentation rulebooks. Where those rulebooks already rule, this one points rather than restates.

## Shape
- A service class per domain area, even when stateless. Methods over free functions.
- Constructor injection of dependencies such as the database pool and the clock. Where a framework offers more than one spelling for injection, the project names one and the reviewer enforces it.
- No function or file size limits. The only split trigger is that a function does two nameable things. Long and direct beats short and indirect.
- Rule of three: a helper earns its existence through actual reuse, or by resolving a two-nameable-things split. No single-use helpers for any other reason, no pass-through layers, and no extraction in anticipation of reuse.
- **A precursor contract slice is not extraction in anticipation of reuse.** Where a shared contract is deliberately landed ahead of its consumers, and those consumers are named, already written and blocked on it, the reuse is scheduled rather than anticipated, so the rule of three does not bind the slice. The distinction is evidence: anticipation is a guess that someone will want this, and a precursor names who is waiting and what breaks meanwhile. A slice claiming this carve-out states its consumers in the pull request description. A slice that does not name them has not claimed the carve-out, and the rule of three binds it as written.
- DRY per the Metz synthesis: deduplicate domain logic on the third occurrence, pure single-purpose utilities on the second, and never deduplicate similar-but-not-same code.

## Style
- Optimize for fewer lines using the language's own expressiveness: comprehensions, concurrent gathers, unpacking, assignment expressions where they stay clear. Density is a virtue only while each line still reads plainly.
- Failing to use an obvious expressive form, such as a loop that is a comprehension or a sequential await chain that is a gather, is a review finding.
- Timezone-aware datetimes only. In Python that is `datetime.now(timezone.utc)`, and `datetime.utcnow()` is a defect, deprecated since 3.12.

## Documentation
Ruled by the code-comments rulebook. This rulebook adds these hunt targets for review:

- Over-documentation is a named defect: a docstring that restates the signature, or a `Parameters` or `Returns` section, is a finding, the same class as a comment restating a name. A public item with no docstring is also a finding; the linter reports it. A private function needs none.
- A comment excusing convoluted code is a smell, not a fix. Truly convoluted code may carry a line or two, but the review first asks why it is convoluted and demands an attempt at simplification before accepting the comment.
- Every function declares full parameter and return types, states its error contract per the error-handling rulebook, and states its expected logging where it logs.
- **A change that alters what a person must run, or where things live, updates the documents that say so, in the same change.** Enumerations of commands, tree roots and gates go stale silently, and a reviewer bound only to the code rulebooks can cite nothing when they do, so the finding arrives as judgement or not at all. In scope is any file that tells a reader which commands to run before pushing, what the tree's roots are, or which gates the build enforces. The test is not whether the document was edited but whether every enumeration it still carries is true afterwards. Where a document and the automated build name the same command, they are one fact stored twice, and a review finding names both copies rather than whichever one it happened to read. The common case, and the worst, is a sentence the change already edits for one reason and leaves stale for another: the line was open and read, so nothing but the habit of re-reading the whole sentence catches it. Prefer an enumeration that states its own maintenance obligation over one that merely lists today's members, and prefer no count at all to a count maintained by hand.

## Consistency across surfaces
A fact the system states in more than one place is one fact stored several times. The places are the code, the API schema and the types generated from it, the wire protocol, configuration examples, the component pages under `docs/`, the README and contributor guide, and the tests.

- A change that alters a fact updates every place that states it, in the same change. The test is whether every statement of the fact is true afterwards, not whether one file was edited.
- A name, a state, a limit or a default has one definition. Every other place derives from it or is checked against it. Prefer generating or testing the copy over maintaining it by hand.
- A review finding on a stale copy names the fact and every place that states it, not only the one the reviewer happened to read.
- This generalises the Documentation rule on commands, tree roots and gates, which stays as written.

## Tests
- Contract and edges: every public method's happy path, every declared error path, and the boundary cases such as empty input, retries, and concurrent updates.
- Real containerized dependencies through a shared fixture. Mock only true externals such as the filesystem, the clock, and the network. Faking the database or mocking the project's own services is a finding. The one exception is fault injection: a test may patch a project method to raise a failure the real dependency cannot be made to produce on demand, delegating to the real method for every other call, and the test's name says which fault it injects.
- One test per contract case, named for the behavior it proves, asserting on behavior and never on query text or internals.
- Tests mirror the source path exactly and never sit beside the source. A project that splits its suite by cost, for example a unit tree that needs no external service and an integration tree that does, mirrors the source path under each.
- **A behaviour ruled wrong is pinned by one test, a known-defect test, and no ordinary test asserts the wrong value.** A behaviour ruled a defect is still pinned, so the cleanup cannot change it by accident, but it is pinned in one place. The test is named `<behaviour>_known_defect` and carries a marker giving a non-empty reason and the id of the open line that tracks the defect. A collection-time check fails the run when a test has the name without the marker, the marker without the name, or a marker missing either field, so the convention cannot decay into a naming habit, and the suite lists the pinned defects on demand. An ordinary test that touches the same record asserts around the wrong value by leaving that field out of its comparison, and never restates it: otherwise the fix turns an unrelated test red and a correct change reads as a regression. When the defect is fixed, the known-defect test becomes an ordinary test of the right behaviour in the same change, and its name and marker go. A behaviour nobody has ruled on is pinned by an ordinary test, so a later ruling has a test to change and not a gap to discover.
- **A test passes alone, in any order, and under any selection.** It builds the state it needs and leaves none behind. Process-wide state such as logging configuration, environment variables, module-level caches and registered handlers is set inside a fixture or test that undoes it, never while a module is imported or collected; building the application, or anything from it, at import time counts, because that runs outside every fixture. A suite that passes only in its default order hides the defect until someone runs one file, a `-k` selection or a parallel worker, and then an unrelated test fails. A reviewer checks a new test file by running it alone and together with a file from another package.

## Pull request shape
- One ticket, one pull request, regardless of diff size.
- Commits cut by logical step so the pull request reads commit by commit.
- Description carries the standing template of summary, problem, approach, why-safe, and how-verified, plus a review map giving a file-by-file suggested reading order with one line each on what to look at.

## Enforcement
The reviewer reports each breach against the section that names it. The simplify stage applies the Style section as its objective function, the build stage applies Shape and Tests, and the pull request draft stage applies Pull request shape. Anything here found wrong in use is amended with a dated note, never silently worked around.

## Project binding
A deriving project states:
- its service class names and its dependency injection spelling,
- its shared test fixtures and what they stand up,
- its source-to-test path mapping.
