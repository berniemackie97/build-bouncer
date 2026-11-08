# BuildBouncer

BuildBouncer runs your project's tests, linters, builds, and other commands before Git pushes. You choose the checks in `.buildbouncer/config.yaml`; BuildBouncer executes them and reports failures. Its local `pre-push` hook runs those checks when you push.

**New here?** Follow [Install](#1-install-buildbouncer) and [Set up your first repository](#2-set-up-your-first-repository). You do not need to clone BuildBouncer to configure it in another project.

> BuildBouncer is a local Git hook, **not** a replacement for trusted CI or branch protection. Anyone can bypass local hooks with `git push --no-verify`. For a push gate that rejects every failed check without an interactive override, configure `protection.level: strict`.

## Contents

- [1. Install BuildBouncer](#1-install-buildbouncer) — macOS, Linux, Windows, and source builds
- [2. Set up your first repository](#2-set-up-your-first-repository) — generate, edit, verify, and install the hook
- [3. Configure checks](#3-configure-checks) — YAML examples and field reference
- [4. Day-to-day usage](#4-day-to-day-usage) — check, push, inspect, and refresh
- [5. Git hooks and protection](#5-git-hooks-and-protection) — enforcement, overrides, and worktrees
- [6. CI workflow import](#6-ci-workflow-import) — supported GitHub Actions translation
- [7. Troubleshooting](#7-troubleshooting) — PATH, missing tools, shell errors, and logs
- [8. Command reference](#8-command-reference) — supported commands and options
- [9. Updating and uninstalling](#9-updating-and-uninstalling) — upgrades and cleanup

## 1. Install BuildBouncer

### Prerequisites

- **Git**, installed and usable from your terminal. The hook requires an existing Git repository.
- **Go for source installations**, with a compatible toolchain (the current `go.mod` specifies Go **1.27** and `toolchain go1.27.3`). Install Go from [go.dev/doc/install](https://go.dev/doc/install), then confirm `go version` works. The source-built Homebrew formula declares Go as a build-only dependency and does not require it at runtime.
- **Your project's own tools**, such as `dotnet`, `npm`, or `go`. BuildBouncer does not install them.

BuildBouncer v0.1.0 is published on [GitHub Releases](https://github.com/berniemackie97/build-bouncer/releases), but its precompiled Homebrew cask fails macOS Gatekeeper and must not be used as an installation method. The next release replaces it with a Homebrew formula that builds from checksummed source. Until that version is published and verified, use the Go installation below. See [release and upgrade guide](docs/releases.md).

### Homebrew formula (available after the next release)

The planned source-build formula installs without the blocked precompiled macOS cask. Once the updated release and tap are published:

```sh
brew install --formula berniemackie97/tap/build-bouncer
"$(brew --prefix)/bin/build-bouncer" --version
```

For existing v0.1.0 cask installations, uninstall that cask **before** installing the formula to avoid a conflicting binary link. If a Go-installed copy takes precedence, run the Homebrew executable by its actual path or adjust PATH deliberately; do not delete unrelated installations automatically.

### macOS / Linux (zsh or Bash)

Run these commands in a terminal **from any directory**:

```sh
go version
go install github.com/berniemackie97/build-bouncer/cmd/build-bouncer@latest

bin_dir="$(go env GOBIN)"
if [ -z "$bin_dir" ]; then
  bin_dir="$(go env GOPATH)/bin"
fi
export PATH="$bin_dir:$PATH"

command -v build-bouncer
build-bouncer --version
```

`go install` puts the executable in `GOBIN` if configured, otherwise in `GOPATH/bin` (usually `~/go/bin`). The `export` above updates **only the current terminal session**. To make the command work in future terminals, add the directory to your shell's startup configuration:

- macOS default zsh: `~/.zshrc`
- Linux Bash: `~/.bashrc` (or your shell's equivalent)

For the **default** Go installation directory, the line to add is:

```sh
export PATH="$HOME/go/bin:$PATH"
```

If `go env GOBIN` or `go env GOPATH` points elsewhere, use the actual `bin_dir` printed by `printf '%s\n' "$bin_dir"` rather than `$HOME/go/bin`. Open a new terminal and run `build-bouncer --version` again. If you use fish or another shell, configure its `PATH` using that shell's syntax.

### Windows (PowerShell)

Open PowerShell (not the Go command prompt). Run:

```powershell
go version
go install github.com/berniemackie97/build-bouncer/cmd/build-bouncer@latest

$binDir = (go env GOBIN)
if (-not $binDir) {
    $binDir = Join-Path (go env GOPATH) 'bin'
}
$env:Path = "$binDir;$env:Path"

Get-Command build-bouncer
build-bouncer --version
```

Go installs `build-bouncer.exe` in the directory shown by `$binDir`. The `$env:Path` change applies to **this PowerShell session only**.

To make it permanent, open Windows **Edit environment variables for your account** → select the user **Path** variable → **Edit** → **New** → enter the directory from `$binDir` (normally `%USERPROFILE%\go\bin`). Open a fresh PowerShell session and verify with `Get-Command build-bouncer`.

Git for Windows is recommended for the Git CLI and the shell used to execute Git hooks. Shells required by individual checks (for example, Git Bash or PowerShell) must also be available on that machine.

### Install from a clone instead

Use this when developing BuildBouncer or installing a specific checkout. This is **not** necessary for normal installation.

```sh
git clone https://github.com/berniemackie97/build-bouncer.git
cd build-bouncer
go install ./cmd/build-bouncer
build-bouncer --version
```

The last command requires the Go bin directory on `PATH` as described above. To run the repository's tests before installing, use `go test ./...`; the project's CI also runs formatting, `go vet ./...`, module verification, and Linux race tests.

## 2. Set up your first repository

These steps run **inside your own project's Git repository**, not the BuildBouncer source checkout. For example, change to your project's root and confirm Git recognizes it:

```sh
cd /path/to/your/project
git rev-parse --show-toplevel
```

Replace `/path/to/your/project` with your actual directory (on Windows, use a path such as `C:\src\my-project`). If Git reports that this is not a repository, open the correct project or initialize Git separately before installing a hook.

### Step 1 — Generate a configuration

Choose **one** template that matches your project:

```sh
build-bouncer init --go
```

Other examples: `build-bouncer init --dotnet` for .NET, `--node` for Node.js, `--python` for Python, `--rust` for Rust, or `--manual` for custom commands. Run `build-bouncer init` **without a template flag** to print the complete list.

Initialization creates:

```text
your-project/
  .buildbouncer/
    config.yaml
    assets/
      insults/default.json
      banter/default.json
```

Initialization uses the selected local template, with safe project discovery where available. For .NET repositories with one discoverable `.sln` or `.slnx`, including nested solutions, it derives a restore → format → build → test sequence targeted to that path. Locked restore is selected only if every discovered project has a regular `packages.lock.json`. Discovery skips generated/dependency directories and symlinks, with bounded traversal. With multiple solutions or none, initialization fails **without writing configuration or assets**; select an explicit repository-relative solution with `build-bouncer init --dotnet --solution services/api/Api.slnx`, or choose a different template/custom checks. A project without a solution is not silently treated as a runnable solution. For mixed repositories, `build-bouncer init --dotnet --node --solution services/api/Api.slnx` selects the .NET solution and only the scripts explicitly declared in the **root** `package.json`. Root pnpm/Turborepo manifests are recognized as context, not evidence that nested or standalone applications belong to the workspace. Generated Node checks use the root package manager's `run` command; they never invent `pnpm --recursive`, Turbo tasks, or package membership. Both profiles must be runnable and unambiguous, or initialization leaves the repository untouched. Node checks are named `node-lint`, `node-tests`, and so on to avoid collisions with .NET checks, and the combined profile runs sequentially because .NET checks have dependencies. Existing GitHub Actions workflows are left untouched and do not prevent setup. To explicitly import supported workflow checks during initialization, pass `--import-ci` (for example, `build-bouncer init --dotnet --import-ci`). An unsupported workflow then causes the requested import to fail without creating a configuration; it is never partially translated or silently treated as covered. The `--manual` template deliberately contains a **failing placeholder check**; replace it with your real commands before running checks.

**Existing configuration is preserved.** `init` refuses to replace `.buildbouncer/config.yaml` (or the legacy `.buildbouncer.yaml`). Do not use `--force` unless you intend to regenerate and replace the configuration and default packs.

### Generated profile validation and hook admission

`build-bouncer validate` checks YAML and known generated targets without executing any configured command. For selected .NET profiles it verifies that the solution is still present and that generated commands match the selected target. It checks root package scripts for generated Node profiles, and checks generated working directories for missing paths and symlink escapes. Legacy root-level .NET template commands require a single root solution or project; otherwise reselect the target using `build-bouncer configure --dotnet --solution PATH --yes`.

`build-bouncer doctor` shows the same static findings alongside executable and prerequisite diagnostics. `build-bouncer hook install` and `setup` refuse to install or replace a hook when a recognized generated configuration is invalid. Refusal leaves an existing hook untouched; setup also restores newly generated configuration and packs if preflight rejects them. User-owned checks (without generated `source` / `id` metadata) remain manually managed: their commands are not executed during validation and directories they may create later are not guessed. When taking ownership of an edited generated check, clear both `source` and `id` and review the command manually.

Static admission cannot certify successful execution, all GitHub Actions behavior, arbitrary shell scripts, or externally installed tools. Run `build-bouncer check --verbose` and trusted repository CI to verify those independently.

### Mixed-workspace execution smoke

The Mixed workspace smoke GitHub Actions workflow creates a temporary Git repository containing a pnpm root workspace, an excluded standalone application, and two nested .NET 10 solutions. It builds BuildBouncer, runs the actual mixed setup and checks, performs a Git push through the installed pre-push hook, and verifies refusal when generated targets become invalid. This runs on Linux, macOS, and Windows.

To run the opt-in integration test locally, install Go, Git, Node 24, pnpm 10.0.0, and .NET 10. Set the environment variable BUILDBOUNCER_REAL_WORKSPACE_SMOKE to 1, then run the TestRealMixedWorkspaceSmoke Go test in cmd/build-bouncer with a suitable timeout. Ordinary Go test runs skip this tool-dependent acceptance test. The public fixture does not substitute for validation of an independent private workspace.

### Step 2 — Review and customize the checks

Use `build-bouncer doctor` to inspect generated commands and prerequisites. `build-bouncer configure` opens an interactive terminal menu to refresh .NET checks, root Node scripts, or both, set protection, or manage custom checks. You can also use scriptable flags:

```sh
build-bouncer configure --dotnet --solution services/api/Api.slnx --protection strict --yes
build-bouncer configure --dotnet --node --solution services/api/Api.slnx --yes
build-bouncer configure --protection strict --yes
build-bouncer configure --parallel 1 --fail-fast true --yes
build-bouncer configure --check lint --run 'dotnet format --verify-no-changes' --yes
build-bouncer configure --remove-check lint --yes
```

For multiple or nested solutions, specify `--solution relative/path/NAME.slnx` with `--dotnet`. Add `--node` to refresh root package scripts alongside .NET, or use `configure --node` to refresh only the Node profile. These operations use ownership IDs to preserve unrelated custom checks and reject edited generated checks. An existing Node-only profile can be converted to mixed checks with `configure --dotnet --node`, without replacing its YAML. Changes to root scripts are not automatically applied until refreshed. Generated .NET restore, formatting, build and test checks run sequentially with fail-fast enabled; an existing `runner.maxParallel` greater than 1 must be explicitly corrected with `--parallel 1`. Without `--yes`, the proposed configuration is shown and must be confirmed. The command updates only owned generated checks, refuses to overwrite modified or user-owned checks, validates before saving, and preserves unrelated YAML fields and comments. Hand-editing `.buildbouncer/config.yaml` remains fully supported for advanced behavior. Review each generated command: templates are starting points, not guarantees that your project has the corresponding scripts, dependencies, or build layout.

For example, a .NET repository might use:

```yaml
version: 1

checks:
  - name: tests
    run: "dotnet test -c Release"
    requires: [dotnet]
  - name: format
    run: "dotnet format --verify-no-changes"
    requires: [dotnet]

runner:
  maxParallel: 2
  failFast: true

protection:
  level: strict
```

This is a **minimal standalone config**; generating a template also sets up optional insult and banter packs. You can retain the generated `insults`, `banter`, and `meta` sections while editing `checks` and `protection`. Adapt the commands to your solution or project. The tools themselves must already be installed.

### Step 3 — Verify before enabling the hook

```sh
build-bouncer validate
build-bouncer doctor
build-bouncer check --verbose
```

- `validate` checks the YAML schema and configuration values.
- `doctor` reports resolved executables, working directories, missing prerequisites, and shell problems.
- `check --verbose` actually runs the commands. Fix failures before installing the hook.

For a non-interactive run, use `build-bouncer check --ci`. If you see a missing tool, install it or correct the check instead of silently permitting the skip.

### Step 4 — Install the pre-push hook

```sh
build-bouncer hook status
build-bouncer hook install
build-bouncer hook status
```

The default install copies the current BuildBouncer executable into Git's effective hooks directory and writes its `pre-push` hook. On your next `git push`, Git invokes that hook and the configured checks run automatically.

BuildBouncer replaces or uninstalls an existing hook without `--force` only when its contents exactly match a generated v4 script or a known, unmodified v3 script. A pristine v3 hook upgrades on reinstall; its old copied executable is retained because another Git worktree may still use it. Foreign hooks, edited scripts and unverified v1/v2 hooks are left unchanged. Inspect them before explicitly using `hook install --force` or `hook uninstall --force`; forcing replacement or removal can discard custom behavior.

`hook status` distinguishes a recognized BuildBouncer hook from a foreign hook, flags modified or outdated installations, and warns when a POSIX hook lacks execute permission.

Copied installations now use `hooks/bin/build-bouncer-<os>-<arch>` (or `.exe` on Windows), selected by the generated hook using the host operating system and CPU architecture. This prevents macOS, Linux and Windows builds from sharing the same executable path. Each copy has a SHA-256 ownership record. Installation refuses to overwrite a missing/unverified record unless explicitly forced, and publishing the hook comes last; partial install/removal failures attempt to restore the previous files. Older v3 copied executables use the original filenames. They are not deleted by v4 upgrades or uninstalls; only uninstalling the actual v3 hook considers removing its verified original executable. Pre-existing copies without records require inspection before forced cleanup. Uninstall never deletes other host copies or unverified temporary files.

For a current copied hook, `hook status` checks executable file permissions and its ownership digest, warning when the copy is missing or altered; the hook may still fall back to `PATH`. With `--no-copy`, status does not inspect copied-file records because the hook resolves `build-bouncer` through `PATH`. A matching digest does not prove executable compatibility or successful execution; status never runs checks.

Hook installation and removal use an atomic `.build-bouncer-hook.lock` directory inside Git's effective hooks directory to prevent two BuildBouncer processes from changing the same hook simultaneously. `hook status` refuses to report a snapshot if it sees this lock. An interrupted process can leave the lock behind; no automatic stale-lock breaking occurs. Before removing it manually, confirm no hook-management process is active and inspect the installed hook, copied executable, checksum, and retained backups. `--force` does not bypass the lock.

When an executable is missing but its valid SHA-256 ownership record remains, ordinary install and uninstall refuse to assume ownership. After inspection, `hook install --force` explicitly repairs the missing copy, or `hook uninstall --force` removes the hook and orphaned record. Malformed or nonregular ownership records are never silently discarded. If a rollback detects an executable modified by another process, BuildBouncer preserves that modification and retains the original staged backup, reporting its path for manual recovery. Multiple-file publication and out-of-band edits are not made atomic by the lock; unknown temporary files are not automatically removed.

### Step 5 — Keep the configuration with your project

Review changes with `git status`. If you want teammates to use the same rules, commit `.buildbouncer/config.yaml` and its desired packs as project files. **Git hooks and copied executables are local** to each clone/worktree setup; teammates still need to install BuildBouncer and run `build-bouncer hook install` themselves.

When your repository has a remote and changes to push, use your usual `git push`. A normal push is blocked when enforced checks fail. Local hooks can be bypassed; enforce required policy in your remote CI as well.

### Short path: configure and run in one command

For a fresh repository with a suitable template:

```sh
build-bouncer setup --go
```

For a .NET repository with one discoverable solution, `build-bouncer setup --dotnet --no-copy` detects its local checks, initializes the configuration, installs a PATH-based hook, and runs checks. For a mixed repository use `build-bouncer setup --dotnet --node --solution services/api/Api.slnx --no-copy`; if discovery fails, it does not install the hook. Setup runs the selected checks after installation, so the required package manager and .NET tools must be available. Run `build-bouncer configure --protection strict --yes` to enforce a strict blocking policy. Review the proposed checks before relying on the hook.

`setup` runs initialization, installs the hook, and immediately executes the checks. A failing check makes `setup` return a failure **but does not undo a successfully installed hook**. If a config already exists, `build-bouncer setup` (without a template selection) uses it; selecting a template again requires explicit replacement using `--force`.

## 3. Configure checks

BuildBouncer reads `.buildbouncer/config.yaml`, or an existing legacy `.buildbouncer.yaml`. Both are YAML; the current schema version is `1`. At least one check is required.

### A check with a subdirectory and environment

```yaml
version: 1
checks:
  - name: frontend-lint
    run: "npm run lint"
    cwd: "src/frontend"
    requires: [npm]
    env:
      CI: "true"
  - name: frontend-tests
    run: "npm test"
    cwd: "src/frontend"
    requires: [npm]
protection:
  level: strict
```

Working directories are relative to the repository root; absolute paths and paths escaping the root are rejected. The check's environment overrides apply only to that check, not to your whole terminal session.

### Common check fields

| Field | Meaning |
| --- | --- |
| `name` | Required unique label in output and reports. |
| `run` | Required command/script to execute. |
| `shell` | Optional executable name or path; default is `cmd.exe` on Windows and `sh` on macOS/Linux. |
| `cwd` | Optional working directory within the repository. |
| `env` | Optional map of environment overrides. |
| `requires` | Optional list of commands that must be available before executing this check. |
| `os` | Optional target OS restriction: `windows`, `linux`, `macos`. |
| `allowMissingTools` | If `true`, permits skipping missing `requires`; an unavailable shell still fails, and a suite with no executed checks fails. |
| `timeout` | Optional check timeout, e.g. `"2m"`; defaults to 30 minutes, maximum 2 hours. |
| `id` / `source` | Metadata on generated checks; `ci sync` owns checks explicitly marked with a `ci:` source. |

Do not set `allowMissingTools: true` for required tests or builds. Missing prerequisite tools otherwise fail closed. If a check applies only to a particular operating system, constrain it with `os` instead of making a required dependency optional. Unknown OS restrictions are rejected rather than treated as unconstrained.

### Runner options

```yaml
runner:
  maxParallel: 2
  failFast: true
  maxLogBytes: 4194304
  maxSavedLogs: 20
  logRetentionDays: 7
```

`maxParallel` allows 1–32 concurrent checks. `failFast` cancels remaining work after a failure. Log settings bound retained failure output; zero uses the built-in defaults.

### Protection policy

```yaml
protection:
  level: strict
```

- `strict`: block a push for any failed/unavailable required check, without interactive override. Recommended for an enforced local quality gate.
- `moderate`: block failures by default, but a push hook can offer an interactive `Push anyway?` override when interactive input is available. This is the default when no level is set.
- `lax`: blocks failures by default, including failures of checks named "lint" or "test". To allow one *executed* check to fail without blocking a local push, explicitly set `allowFailure: true` on that check. Missing required tools, invalid prerequisites, timeouts, canceled checks, and unexecuted suites still block. Use `strict` for an enforced gate.

`protection.interactive: false` disables the interactive override in non-strict modes. `check --ci` always blocks failures, including `allowFailure: true` checks, regardless of the configured protection level. The old `protection.criticalPatterns` field was never enforced and is now rejected instead of silently ignored; use per-check `allowFailure` for explicit lax-mode exceptions.

For an explicitly non-blocking local check:

```yaml
checks:
  - name: formatting-report
    run: "npm run lint"
    allowFailure: true
protection:
  level: lax
```

The check still runs, appears in failure reports, and fails CI. The exception applies only to local lax-mode enforcement. Renaming a check never changes whether its failure blocks a push.

### Choosing a shell

`shell` is **one executable**, not an executable plus flags:

```yaml
checks:
  - name: posix-tests
    shell: bash
    run: "go test ./..."
    os: [linux, macos]
  - name: windows-tests
    shell: pwsh
    run: "dotnet test -c Release"
    os: [windows]
```

Quote an executable path with spaces (for example, `shell: '"C:\Program Files\PowerShell\7\pwsh.exe"'`). Relative executable paths containing a directory separator resolve from the repository root, independently of `cwd`. The configured executable is never silently replaced with another shell.

To check a Windows WSL installation independently, run `wsl.exe --list --verbose` and `wsl.exe --exec sh -c "printf ready"` in PowerShell. An installed WSL launcher alone does not guarantee an initialized Linux distribution. The Windows integration regression tests exercise the configured WSL shell only when a usable default distribution is available.

If `shell` is omitted, `run` may also use a recognized direct invocation such as `bash -lc "go test ./..."`, `sh -c 'echo ready'`, or PowerShell's `-Command`. Proper quoting is required for direct POSIX scripts; malformed recognized invocations fail instead of silently falling back to a different interpreter. Other command strings use the OS default shell.

On Windows, a bare `bash` or `sh` prefers a compatible Git Bash/MSYS installation, not an implicit WSL launcher. To use WSL intentionally, select `shell: wsl.exe`; BuildBouncer invokes `wsl.exe --exec sh -c`, requiring an installed and initialized Linux distribution. Per-check `env` overrides are rejected for WSL launchers because they are not automatically forwarded into the Linux environment. Live initialized-distribution compatibility remains an open verification item.

### Personality packs

Initialization creates editable JSON assets at `.buildbouncer/assets/insults/default.json` and `.buildbouncer/assets/banter/default.json`. The generated config points to them:

```yaml
insults:
  mode: snarky           # polite | snarky | nuclear
  file: ".buildbouncer/assets/insults/default.json"
  locale: en
banter:
  enabled: true
  file: ".buildbouncer/assets/banter/default.json"
  locale: en
```

Set `banter.enabled: false` for neutral progress messaging. You can replace the file paths with your own compatible packs.

## 4. Day-to-day usage

| Goal | Command |
| --- | --- |
| Run all checks | `build-bouncer check` |
| See complete output and failure details | `build-bouncer check --verbose` |
| Run without interactive UI/banter | `build-bouncer check --ci` |
| Stop remaining checks after a failure | `build-bouncer check --fail-fast` |
| Run up to four checks concurrently | `build-bouncer check --parallel 4` |
| Validate edited configuration | `build-bouncer validate` |
| Diagnose shell and prerequisites | `build-bouncer doctor` |
| Check the active pre-push hook | `build-bouncer hook status` |
| Push normally (with the hook) | `git push` |

Run these from your project or a subdirectory where configuration discovery can find the repository's config file.

When tests fail, fix them and rerun `check --verbose`. Successful checks do not retain their logs; failed checks normally write logs under the Git-resolved `build-bouncer/logs` state directory (commonly `.git/build-bouncer/logs`). The default retained output is bounded (4 MiB per log, up to 20 saved logs, 7-day retention). Tool output can contain secrets; avoid emitting credentials into logs.

With `--verbose` or `--ci`, runner progress and subprocess output use the command's output stream and are serialized across concurrently running checks. Interactive spinner rendering is synchronized with shutdown, including repeated stop requests. Optional banter history is stored in a private state file; concurrent processes coordinate writes through a short-lived `.lock` directory beside that file. An interrupted process may leave the lock behind, in which case persistence reports the lock's path for manual inspection rather than discarding another process's data. Corrupt history is preserved and reported instead of being silently reset.

## 5. Git hooks and protection

`build-bouncer hook install` writes the `pre-push` hook to Git's **effective** hooks directory. This handles normal repositories, linked worktrees, and Git's `core.hooksPath` setting; the path is not necessarily the visible `.git/hooks` directory.

By default, it copies the current executable into `<effective-hooks-directory>/bin/build-bouncer` (or `build-bouncer.exe`). The hook prefers that copied binary. This means **updating a global BuildBouncer installation does not automatically update the copy in each hook**.

Use `build-bouncer hook install --no-copy` only when you deliberately want the hook to resolve `build-bouncer` from `PATH` at push time. If a pre-existing hook was installed by another tool, inspect it before replacing it; `--force` is destructive to that hook.

To remove just the hook while leaving project configuration alone:

```sh
build-bouncer hook uninstall
```

**Security boundary:** `git push --force` still runs the hook. `git push --no-verify` skips it, so remote CI and branch protection are necessary for server-side enforcement. Configuration contains executable commands; review it before running checks from an untrusted repository.

## 6. CI workflow import

Local initialization does not import GitHub Actions automatically. Use `build-bouncer init --go --import-ci` or `build-bouncer setup --go --import-ci` to request import when creating a config. To import into an existing configuration or refresh previously imported checks:

```sh
build-bouncer ci sync
build-bouncer validate
build-bouncer check --ci
```

Import supports literal scripts, supported OS runners and matrices, working-directory defaults, scalar environment values, explicit shells, and certain runner OS conditions. Unsupported workflow expressions, action semantics, or ambiguous matrix configurations are rejected rather than guessed.

`setup --ci` remains non-interactive check execution; it does not request workflow import. When using `setup --import-ci`, select a template; use `ci sync` to import into an existing configuration. `ci sync` replaces only checks explicitly owned by `source: ci:...` and preserves manually authored checks. Imported checks keep their OS restriction, so Windows checks are skipped when running locally on macOS/Linux, and vice versa. Setup/cache actions are **not** replayed locally: install required tools yourself.

This is not a complete GitHub Actions executor. Keep repository CI as the authoritative check.

## 7. Troubleshooting

### `build-bouncer: command not found`

The Go executable directory is missing from `PATH`. On macOS/Linux run `go env GOBIN` and `go env GOPATH`, then `command -v build-bouncer`. On Windows inspect `$binDir` from the installation section and run `Get-Command build-bouncer`. Make the PATH update permanent and open a new terminal.

### `go install` fails

Check `go version`, network access, and the module's Go toolchain requirement. If building a local checkout, run `go install ./cmd/build-bouncer` from the **BuildBouncer source repository** instead of your application repository.

### `configuration not found`

Run `build-bouncer init --go` (or your template) in the intended project. Confirm `.buildbouncer/config.yaml` exists. `check`, `validate`, and `doctor` search upward from the current directory; running them in an unrelated directory cannot find another project's config.

### Missing tool or shell; wrong interpreter

Run `build-bouncer doctor` to see the executable selection and missing `requires`, then `build-bouncer check --verbose` for execution errors. Install the missing dependency or correct `shell`/`requires`/`cwd`. On Windows, Bash checks need a usable Bash installation; selecting a nonexistent shell is an error, not an optional skip.

### `git push` does not run checks

From the Git repository, run `build-bouncer hook status`. Install with `build-bouncer hook install` if absent. Check the reported hook path, Git's `core.hooksPath` setting, and whether the hook's copied binary is present. Git does not transfer your local hook installation to teammates.

### A push is blocked after setup

`setup` installs the hook before it executes the initial check run. Fix the failing check, rerun `build-bouncer check --verbose`, then push again. Changing `.buildbouncer/config.yaml` requires validation; changes to the CLI itself require refreshing copied hooks if you want the newer binary.

### I want a clean removal

Use `build-bouncer uninstall --dry-run` to inspect hook removal first. `build-bouncer uninstall` preserves configuration and state; `build-bouncer uninstall --purge --dry-run` previews permanent cleanup and `build-bouncer uninstall --purge --yes` confirms it. Purging deletes user-edited packs and configuration.

## 8. Command reference

| Command | Purpose |
| --- | --- |
| `build-bouncer --version` / `build-bouncer help` | Display version or available commands. |
| `build-bouncer init --go [--import-ci]` | Generate a Go config and default assets; import supported CI checks only when requested (other template flags supported). |
| `build-bouncer init` | List supported templates and aliases. |
| `build-bouncer setup --go [--import-ci]` | Initialize, install the hook, and run checks; CI import requires explicit opt-in. |
| `build-bouncer check [--verbose] [--ci] [--parallel N] [--fail-fast]` | Run checks with optional diagnostic, CI, concurrency, and cancellation modes. |
| `build-bouncer validate [--config PATH]` | Validate the selected or discovered config. |
| `build-bouncer doctor [--config PATH]` | Inspect shell selection, working directories, and prerequisites. |
| `build-bouncer ci sync` | Refresh supported GitHub Actions checks. |
| `build-bouncer configure [--dotnet] [--protection LEVEL] [--yes]` | Interactively or noninteractively manage local checks and protection without hand-editing YAML. |
| `build-bouncer hook install [--no-copy] [--force]` | Install the pre-push hook. |
| `build-bouncer hook status` | Inspect the active hook and copied binary. |
| `build-bouncer hook uninstall [--force]` | Remove the hook, requiring force for another tool's hook. |
| `build-bouncer uninstall [--dry-run] [--purge --yes] [--force]` | Remove the hook, optionally purge project data. |

Additional check flags: `--hook` is used by the installed pre-push hook; `--log-dir DIR` changes where failed logs are saved; `--tail N` requests extra failure tail lines in verbose mode. `--parallel` accepts 1–32 as an explicit value. `init --force` and `setup --force` replace existing managed configuration; use them only after reviewing what will be replaced.

Exit codes: `0` means success, `2` indicates a configuration/usage error, and `10` indicates checks failed or were canceled. Protection policy can intentionally permit some failures in non-strict modes; configure `strict` for a blocking quality gate.

## 9. Updating and uninstalling

### Upgrade the global executable

For Go-based development installations, run `go install ...@latest` as in [Install](#1-install-buildbouncer) and verify the module revision with `go version -m "$(command -v build-bouncer)"`. Published Homebrew installations should be upgraded using Homebrew; see [Release and update guide](docs/releases.md). Source builds identify themselves as `dev`, while tagged releases include version and commit metadata.

**Important:** If a project's hook was installed with the default copied executable, run the following **from each affected project** to refresh the copied binary:

```sh
build-bouncer hook install
build-bouncer hook status
```

The same applies to other team members' Git clones. If you used `--no-copy`, the hook resolves the global executable at execution time instead.

### Uninstall from a repository

```sh
build-bouncer uninstall --dry-run
build-bouncer uninstall
```

This preserves `.buildbouncer/` and existing state. To preview a full purge:

```sh
build-bouncer uninstall --purge --dry-run
```

Only when you want to permanently delete the project's config, packs, and BuildBouncer state, run `build-bouncer uninstall --purge --yes`. These operations do **not** remove the globally installed executable; uninstall that separately through your Go bin directory if desired.

## Engineering, roadmap, and license

See [engineering standards](docs/engineering.md) and the [modernization roadmap](https://github.com/berniemackie97/build-bouncer/issues/16). Native release packaging and publication requirements are documented in [Release and update guide](docs/releases.md). BuildBouncer is licensed under the [MIT License](LICENSE). The Homebrew tap, publication credentials, and initial release are still outstanding; see [release engineering issue #13](https://github.com/berniemackie97/build-bouncer/issues/13). Broader CI workflow execution remains separate work.
