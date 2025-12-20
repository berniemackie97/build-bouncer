# build-bouncer

[![wakatime](https://wakatime.com/badge/user/dd1b4aee-71e4-41d0-a2d8-e1b2a0865b30/project/156385c5-7934-4448-b729-513c8a1fab43.svg)](https://wakatime.com/badge/user/dd1b4aee-71e4-41d0-a2d8-e1b2a0865b30/project/156385c5-7934-4448-b729-513c8a1fab43)
A terminal bouncer for your repo: runs your checks and blocks `git push` when things fail.

It’s intentionally dumb in the right way: **it does not guess your build system** (yet). You tell it what to run, it runs it consistently, and it enforces it at the gate.

---

## What it does (today)

### Core behavior
- Reads a repo config: **`.buildbouncer.yaml`**
- Runs your configured checks (tests, lint, build, whatever you want)
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
- **Quiet mode (default):** banter + spinner + minimal output
- **Hook mode (`--hook`):** same as quiet mode, even if Git/hook output doesn’t look like a “real terminal”
- **Verbose mode (`--verbose`):** streams the full tool output (like a CI transcript)
- **CI mode (`--ci`):** no spinner/banter, no random insult

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
- `.buildbouncer.yaml`
- `assets/insults/default.json`
- `assets/banter/default.json`

It populates those from templates shipped with build-bouncer:
- `assets/templates/insults_default.json`
- `assets/templates/banter_default.json`

`init` always writes `.buildbouncer.yaml` (overwriting if it already exists).
`--force` overwrites existing default packs.

If `.github/workflows/*.yml` exists, `init` adds each `run` step as a check and skips duplicates.

If no template flag is provided, `init` prints the list of supported templates.

Currently supported templates:
- Manual: `--manual`, `--custom`, `--blank`
- Go: `--go`, `--golang`
- .NET: `--dotnet`, `--net`
- Node: `--node`, `--nodejs`, `--js`, `--javascript`
- React: `--react`, `--reactjs`
- Vue: `--vue`, `--vuejs`
- Angular: `--angular`, `--ng`
- Svelte: `--svelte`, `--sveltekit`
- Next.js: `--next`, `--nextjs`
- Nuxt: `--nuxt`, `--nuxtjs`
- Astro: `--astro`
- Python: `--python`, `--py`
- Ruby: `--ruby`, `--rails`
- PHP: `--php`, `--laravel`
- Java (Maven): `--maven`, `--java-maven`
- Java (Gradle): `--gradle`, `--java-gradle`
- Kotlin: `--kotlin`, `--kt`
- Android: `--android`
- Rust: `--rust`
- C/C++: `--cpp`, `--cxx`, `--cplusplus`
- Swift: `--swift`
- Flutter: `--flutter`
- Dart: `--dart`
- Elixir: `--elixir`

The manual template includes a placeholder check that fails until you replace it.

### `build-bouncer check [--hook] [--verbose] [--ci] [--log-dir DIR] [--tail N]`
Runs all configured checks.

Flags:
- `--verbose` : stream full output to terminal (still logs)
- `--ci` : disables spinner/banter + disables random insults
- `--hook` : forces spinner/banter even if stdout doesn’t look like a TTY (used by the git hook)
- `--log-dir` : override log directory (default: `.git/build-bouncer/logs`)
- `--tail` : number of lines printed per failed check (default: `30`)

Exit codes:
- `0` success
- `2` usage/config error
- `10` checks failed (push blocked)

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
- Default behavior refuses to delete a hook it didn’t install.
- `--force` removes it anyway.

---

## Configuration (`.buildbouncer.yaml`)

Example:

```yaml
version: 1

checks:
  - name: "tests"
    run: "go test ./..."
  - name: "lint"
    run: "go vet ./..."

insults:
  mode: "snarky"   # polite | snarky | nuclear
  file: "assets/insults/default.json"
  locale: "en"

banter:
  file: "assets/banter/default.json"
  locale: "en"
```

Each check:
- `name`: label shown in output and failure summary
- `run`: command string executed via shell (`cmd.exe /C` on Windows, `sh -c` elsewhere)
- optional:
  - `cwd`: run relative to repo root
  - `env`: key/value env vars for just that check

Example with `cwd` + `env`:

```yaml
checks:
  - name: "web:lint"
    cwd: "src/web"
    run: "npm run lint"
    env:
      CI: "true"
```

---

## Insults pack (JSON)

Configured via:

```yaml
insults:
  file: "assets/insults/default.json"
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
  file: "assets/banter/default.json"
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

### “Quiet mode is too quiet”
Use:
- `build-bouncer check --verbose`

Or for more failure output without full streaming:
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

- Improve GitHub Actions mirroring (matrices, OS filtering, and non-run steps)
- Smarter error "headline" extraction per tool (human-readable summaries)
- Packaging: Homebrew, Scoop, Winget, etc.
- TUI configuration editor (optional)

---

## License
MIT
