package hooks

func renderPrePushHook(hasCopiedBinary bool) string {
	return renderHook(hasCopiedBinary, "v4", copiedBinarySelection)
}

func renderLegacyPrePushHook(hasCopiedBinary bool) string {
	return renderHook(hasCopiedBinary, "v3", legacyCopiedBinarySelection)
}

const copiedBinarySelection = `
case "$(uname -s 2>/dev/null)" in
  Darwin) platform=darwin ;;
  Linux) platform=linux ;;
  MINGW*|MSYS*|CYGWIN*) platform=windows ;;
  *) platform="" ;;
esac
case "$(uname -m 2>/dev/null)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  i?86) arch=386 ;;
  armv6*|armv7*) arch=arm ;;
  ppc64le) arch=ppc64le ;;
  s390x) arch=s390x ;;
  riscv64) arch=riscv64 ;;
  loongarch64) arch=loong64 ;;
  *) arch="" ;;
esac
if [ -n "$platform" ] && [ -n "$arch" ]; then
  candidate="$hook_dir/bin/build-bouncer-$platform-$arch"
  if [ "$platform" = "windows" ]; then
    candidate="$candidate.exe"
  fi
  if [ -x "$candidate" ]; then
    bb="$candidate"
  fi
fi
`

const legacyCopiedBinarySelection = `
case "$(uname -s 2>/dev/null)" in
  MINGW*|MSYS*|CYGWIN*)
    if [ -x "$hook_dir/bin/build-bouncer.exe" ]; then
      bb="$hook_dir/bin/build-bouncer.exe"
    fi
    ;;
  *)
    if [ -x "$hook_dir/bin/build-bouncer" ]; then
      bb="$hook_dir/bin/build-bouncer"
    fi
    ;;
esac
`

const hookDispatch = `
if [ -z "$bb" ]; then
  if command -v build-bouncer >/dev/null 2>&1; then
    bb="build-bouncer"
  else
    echo "build-bouncer: not found. Install it or run: build-bouncer hook install" 1>&2
    exit 1
  fi
fi

if [ -t 0 ]; then
  "$bb" check --hook
elif ( : </dev/tty ) 2>/dev/null; then
  "$bb" check --hook < /dev/tty
else
  "$bb" check --hook
fi
`

func renderHook(hasCopiedBinary bool, version, selection string) string {
	body := "#!/bin/sh\n# build-bouncer pre-push hook " + version + "\n" + `set -eu

hook_dir="$(CDPATH= cd -P "$(dirname "$0")" && pwd)"
repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$repo_root" || exit 1

bb=""
`
	if hasCopiedBinary {
		body += selection
	}
	return body + hookDispatch
}
