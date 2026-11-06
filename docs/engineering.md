# Engineering standards and delivery workflow

BuildBouncer is a cross-platform developer tool that runs configured quality checks and enforces their outcomes through Git hooks. Preserve existing behavior unless a confirmed defect or an explicitly planned compatibility change requires otherwise.

## Product guarantees

- A passing result means the applicable required checks executed and passed. Unavailable mandatory prerequisites, execution errors, and empty applicable suites must block.
- Report the real status of every check. Never silently treat unavailable dependencies as successful checks.
- Git hooks offer local feedback; they are bypassable. Trusted CI and repository branch protections are the final enforcement boundary.
- Treat repository configuration and imported workflows as executable input. Validate boundaries and do not claim full CI equivalence for unsupported workflow features.
- Prevent accidental data loss during initialization, hook installation, synchronization, upgrades, and uninstall.
- Bound runtime concurrency, execution time, output memory, and persistent log storage. Stop and reap subprocesses when canceled.
- Keep errors actionable without exposing environment secrets or needless sensitive data.

## Code and architecture

- Prefer idiomatic Go, cohesive packages, explicit ownership, and straightforward control flow.
- Keep the CLI focused on orchestration, config on parsing and validation, runner on execution and results, hooks on Git integration, and presentation on formatting.
- Consolidate duplicated policy implementations; do not copy shell, OS, or Git-path resolution into parallel helpers.
- Add interfaces only for an actual seam, not as generic wrappers around concrete code.
- Reject gratuitous abstractions, speculative scaffolding, repeated boilerplate, catch-all error suppression, unreachable branches, redundant checks, and comments that restate code.
- Use names that describe behavior. Preserve necessary compatibility and security constraints; do not remove safeguards simply to shorten an implementation.
- Profile before performance refactoring. Do not introduce caches, pooling, or concurrency machinery without a demonstrated need.

## Evidence and quality gates

- Before editing, refresh the default branch and existing PR state. Read the relevant source files, callers, tests, issue criteria, and documentation.
- Reproduce or characterize the behavior being changed. Distinguish confirmed defects from hypotheses and optional product features.
- Implement cohesive slices: audit, design, implement, regression tests, documentation, re-audit, correct findings, verify CI, and present the PR.
- Test externally observable behavior, negative paths, cancellation, partial failures, timeouts, concurrent work, and actual Git operations where relevant.
- Validate with `gofmt`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and applicable multi-platform integration checks.
- Inspect GitHub Actions on the current HEAD, not only prior successful runs. Compilation alone is not acceptance.
- Re-audit every slice for duplication, responsibility boundaries, uncontrolled resources, hidden failures, shallow tests, and accidental behavior changes.
- Document verified behavior and compatibility requirements concisely; do not write speculative narratives.

## GitHub delivery

- Track milestones and micro-goals under [the modernization roadmap](https://github.com/berniemackie97/build-bouncer/issues/16).
- Work on an existing relevant PR when practical; otherwise use a focused branch and PR. Never modify the default branch directly.
- Inspect branch HEAD before writes, preserve unrelated work, and avoid force updates.
- Use professional imperative commit messages without decorative or attribution footers.
- Correct legitimate review findings and resolve addressed threads without unnecessary replies.
- Leave PRs open for maintainer review and merging. Close issues only after acceptance criteria are verified and the change is merged.
- Prefer complete, meaningful slices over many trivial commits, and avoid unrelated refactors.
