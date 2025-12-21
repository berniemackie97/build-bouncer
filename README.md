# build-bouncer

[![wakatime](https://wakatime.com/badge/user/dd1b4aee-71e4-41d0-a2d8-e1b2a0865b30/project/156385c5-7934-4448-b729-513c8a1fab43.svg)](https://wakatime.com/badge/user/dd1b4aee-71e4-41d0-a2d8-e1b2a0865b30/project/156385c5-7934-4448-b729-513c8a1fab43)

---
A terminal bouncer for your repo: runs your checks and blocks `git push` when things fail.

It’s intentionally dumb in the right way: **it does not guess your build system** (yet). You tell it what to run, it runs it consistently, and it enforces it at the gate.

---

## What it does (today)

### Core behavior
- Reads a repo config: **`.buildbouncer/config.yaml`**
- Keeps build-bouncer config + packs under `.buildbouncer/` (legacy `.buildbouncer.yaml` is still supported)
- Runs your configured checks (tests, lint, build, whatever you want)
- Generated checks include stable IDs + source metadata for reproducible re-syncs
- If any check fails:
  - **blocks the push** (exit code `10`)
  - prints a clean summary of failed checks
  - prints a readable tail of output
  - writes the full output to a log file you can open
  - optionally drops a context-aware insult

### Git hook integration
- Installs a **`pre-push`** git hook that runs: `build-bouncer check --hook`
- By default it **copies the build-bouncer binary into your repo** at:
  - `.git/hooks/bin/build-bouncer(.exe)`
- The hook prefers that repo-pinned binary first (so it doesn’t accidentally run some other global version on your PATH).

### Output modes
- **Quiet mode (default):** banter + spinner + one-line failure output (insult + check/location)
- **Hook mode (`--hook`):** same as quiet mode, even if Git/hook output doesn't look like a "real terminal"
- **Verbose mode (`--verbose`):** streams the full tool output + shows per-check "why it failed"
- **CI mode (`--ci`):** no spinner/banter, no random insult
- Skipped checks (missing tools or OS mismatch) show in verbose/CI output.

### Customizable personality (all external files)
- **Insults pack** (JSON): weighted, category-aware (`tests/lint/build/ci/any`), cooldown/no-repeat, supports placeholders like `{check}`, `{checks}`, `{detail}`
- **Banter pack** (JSON): intro/loading/success lines, weighted + cooldown/no-repeat

State for “don’t repeat yourself” is saved under `.git/` so it doesn’t get committed:
- `.git/build-bouncer/insults_state.json`
- `.git/build-bouncer/banter_state.json`

Logs also default under `.git/`:
- `.git/build-bouncer/logs/*.log`

---

## Install

### From source (right now)
This repo is Go. Build a binary and put it somewhere on your PATH.

Example (Windows / PowerShell):

```powershell
$ErrorActionPreference = "Stop"
go build -o build-bouncer.exe .\cmd\build-bouncer
```

Or install into your Go bin:

```powershell
$ErrorActionPreference = "Stop"
go install .\cmd\build-bouncer
```

> Packaging targets (Homebrew/Scoop/etc) are planned, but this README documents what exists today.

---

## Quick start

From your repo root:

```bash
build-bouncer init --go
build-bouncer hook install
```

Or do it all in one:

```bash
build-bouncer setup --go
```

Now pushing runs checks automatically:

```bash
git push
```

---

## Commands

### `build-bouncer init [--force] [--template-flag]`
Creates:
- `.buildbouncer/config.yaml`
- `.buildbouncer/assets/insults/default.json`
- `.buildbouncer/assets/banter/default.json`

It populates those from templates shipped with build-bouncer:
- `assets/templates/insults_default.json`
- `assets/templates/banter_default.json`

`init` always writes `.buildbouncer/config.yaml` (overwriting if it already exists).
`--force` overwrites existing default packs.

If `.github/workflows/*.yml` exists, `init` adds each `run` step as a check and skips duplicates.
For Node-based templates, `init` reads `package.json` scripts (npm/yarn/pnpm/bun) and only includes checks that exist.
Python templates prefer Poetry/PDM/Pipenv/uv/rye/hatch runners when detected.
Gradle/Maven templates prefer wrapper scripts (`gradlew`/`mvnw`) when present.
Rust templates respect `rust-toolchain*` component lists: fmt/clippy checks are included only if the component is listed.

If no template flag is provided, `init` prints the list of supported templates.

Run `build-bouncer init` (no flags) to see the full template list and flag aliases.
Common templates include Manual, Go, .NET, Node (React/Vue/Angular/Svelte/Next/Nuxt/Astro),
Python, Ruby, PHP, Java (Maven/Gradle), Kotlin/Android, Rust, C/C++, Swift, Flutter, Dart,
Elixir, Deno, Scala, Clojure, Haskell, Erlang, Lua, Perl, R, and Terraform.

The manual template includes a placeholder check that fails until you replace it.

### `build-bouncer check [--hook] [--verbose] [--ci] [--log-dir DIR] [--tail N] [--parallel N] [--fail-fast]`
Runs all configured checks.

Flags:
- `--verbose` : stream full output to terminal (still logs)
- `--ci` : disables spinner/banter + disables random insults
- `--hook` : forces spinner/banter even if stdout doesn't look like a TTY (used by the git hook)
- `--log-dir` : override log directory (default: `.git/build-bouncer/logs`)
- `--tail` : extra tail lines printed per failed check in verbose mode (default: `0`)
- `--parallel` : max concurrent checks (default: 1 or config)
- `--fail-fast` : cancel remaining checks after the first failure

Exit codes:
- `0` success
- `2` usage/config error
- `10` checks failed (push blocked)

### `build-bouncer validate [--config PATH]`
Validates `.buildbouncer/config.yaml` and prints the number of checks.

Use `--config` to validate a specific file instead of searching from the current directory.

### `build-bouncer doctor [--config PATH]`
Prints resolved shell/cwd, PATH, and missing tools per check.

### `build-bouncer setup [--force] [--no-copy] [--ci] [--template-flag]`
Convenience: init (if needed) + install hook + run checks.

- `--force` overwrites default packs
- `--no-copy` installs hook without copying the binary into `.git/hooks/bin`
- `--ci` runs checks in CI mode
- Template flags choose a template when generating config (see list above)

### `build-bouncer hook install [--no-copy]`
Installs `.git/hooks/pre-push`.

- Default: copies build-bouncer into `.git/hooks/bin/`
- With `--no-copy`: relies on a globally installed `build-bouncer` on PATH

### `build-bouncer hook status`
Reports whether the hook exists, whether build-bouncer installed it, and whether a copied binary is present.

### `build-bouncer hook uninstall [--force]`
Removes the hook.
- Default behavior refuses to delete a hook it didn't install.
- `--force` removes it anyway.

### `build-bouncer uninstall [--force]`
Removes build-bouncer artifacts from the repo, including:
- `.buildbouncer/`
- `.buildbouncer.yaml` (legacy)
- `.git/build-bouncer/`
- the pre-push hook

### `build-bouncer ci sync`
Refreshes `ci:` checks from `.github/workflows/*` `run` steps, removes stale CI entries, and skips duplicates against your custom checks.
Setup actions like `actions/setup-node`/`setup-go`/`setup-python` are mirrored as lightweight checks (ex: `node --version`), and `setup-node` uses `cache` hints to pick npm/yarn/pnpm.

---

## Configuration (`.buildbouncer/config.yaml`)

Legacy configs at `.buildbouncer.yaml` are still read if present.

Example:

```yaml
version: 1
meta:
  template:
    id: "go"
  inputs:
    node.runner: "pnpm"

checks:
  - name: "tests"
    id: "template:go:9c1f5a9a7c3f"
    source: "template:go"
    run: "go test ./..."
    requires: ["go"]
  - name: "lint"
    id: "template:go:1f9a2bc1230d"
    source: "template:go"
    run: "go vet ./..."
  - name: "build"
    id: "template:go:3e0a11ff9abc"
    source: "template:go"
    run: "go build ./..."
    timeout: "2m"

runner:
  maxParallel: 4
  failFast: true

insults:
  mode: "snarky"   # polite | snarky | nuclear
  file: ".buildbouncer/assets/insults/default.json"
  locale: "en"

banter:
  file: ".buildbouncer/assets/banter/default.json"
  locale: "en"
```

Each check:
- `name`: label shown in output and failure summary
- `id`: stable identifier (generated)
- `source`: where the check came from (`template:*`, `ci:*`, `detector:*`)
- `run`: command string executed via shell (`cmd.exe /C` on Windows, `sh -c` elsewhere)
- optional:
  - `shell`: override shell (examples: `bash`, `sh`, `powershell`, `pwsh`, `cmd`)
  - `cwd`: run relative to repo root
  - `env`: key/value env vars for just that check
  - `os` / `platforms`: restrict to specific OS (`windows`, `linux`, `macos`)
  - `requires`: binaries required to run the check (missing tools will be skipped)
  - `timeout`: per-check timeout (example: `30s`, `2m`)

`shell` must be just the executable name or path (no arguments). Use `run` for the actual command.

If `shell` is omitted, build-bouncer uses the OS default (`cmd` on Windows, `sh` on macOS/Linux).
Generated checks include explicit shells based on CI defaults or script type; for manual checks, set `shell` if you need `bash` or `pwsh`.

Example with `cwd` + `env`:

```yaml
checks:
  - name: "web:lint"
    cwd: "src/web"
    run: "npm run lint"
    env:
      CI: "true"
```

Runner options (optional):
- `runner.maxParallel`: maximum concurrent checks
- `runner.failFast`: cancel remaining checks after the first failure

---

## Insults pack (JSON)

Configured via:

```yaml
insults:
  file: ".buildbouncer/assets/insults/default.json"
```

Format supports:
- categories: `tests`, `lint`, `build`, `ci`, `any`
- weights (chance)
- cooldown (don’t repeat recently)
- placeholders:
  - `{check}` = one failing check name
  - `{checks}` = comma list of all failing checks
  - `{detail}` = best-effort extracted detail (ex: failing test name or first error)

You can ship your own pack and point config at it.

---

## Banter pack (JSON)

Configured via:

```yaml
banter:
  file: ".buildbouncer/assets/banter/default.json"
```

Types:
- `intro` : printed once before checks
- `loading` : used while spinner runs
- `success` : printed once if everything passed

This is what makes quiet mode feel like a bouncer instead of a CI log.

---

## Logs & debugging

### Where logs go
By default:
- `.git/build-bouncer/logs/*.log`

Logs are only kept for failed checks. Successful checks delete their temp log.

### "Quiet mode is too quiet"
Use:
- `build-bouncer check --verbose`

If you want extra tail lines after verbose output:
- `--tail 80`

### Bypass (standard git behavior)
A user can bypass local hooks with:

```bash
git push --no-verify
```

So: **use build-bouncer for fast local feedback**, and still rely on CI as the final authority.

---

## Example configs for common stacks

### .NET
```yaml
checks:
  - name: "tests"
    run: "dotnet test -c Release"
  - name: "format"
    run: "dotnet format --verify-no-changes"
```

### Node / React / Astro
```yaml
checks:
  - name: "lint"
    run: "npm run lint"
  - name: "tests"
    run: "npm test"
  - name: "build"
    run: "npm run build"
```

### Rust
```yaml
checks:
  - name: "fmt"
    run: "cargo fmt --check"
  - name: "clippy"
    run: "cargo clippy -- -D warnings"
  - name: "tests"
    run: "cargo test"
```

### C/C++
```yaml
checks:
  - name: "configure"
    run: "cmake -S . -B build"
  - name: "build"
    run: "cmake --build build"
  - name: "tests"
    run: "ctest --test-dir build"
```

---

## How the git hook works

When you run:

```bash
build-bouncer hook install
```

It writes `.git/hooks/pre-push` that runs:

```bash
build-bouncer check --hook
```

And if CopySelf is enabled (default), it copies the current executable into:

- `.git/hooks/bin/build-bouncer` (mac/linux)
- `.git/hooks/bin/build-bouncer.exe` (windows)

The hook prefers that repo-pinned binary first, so everyone on the team gets consistent behavior per repo.

---

## Roadmap (not implemented yet)

- Expand GitHub Actions mirroring (more step types like `uses`, services, caching)
- Broaden error headline extraction across more tools
- Packaging: Homebrew, Scoop, Winget, etc.
- TUI configuration editor (optional)

---

## License
MIT

