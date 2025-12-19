# build-bouncer

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
- **Hook mode:** same as quiet mode, even if Git/hook output doesn’t look like a “real terminal”
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
