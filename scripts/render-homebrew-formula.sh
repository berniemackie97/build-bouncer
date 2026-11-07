#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 5 ]; then
  echo "usage: render-homebrew-formula.sh VERSION COMMIT SOURCE_ARCHIVE SOURCE_URL OUTPUT" >&2
  exit 2
fi

version="$1"
commit="$2"
archive="$3"
url="$4"
output="$5"

if [[ ! "$version" =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "invalid formula version: $version" >&2
  exit 1
fi
if [[ ! "$commit" =~ ^[0-9a-f]{40}$ ]]; then
  echo "invalid source commit" >&2
  exit 1
fi
if [ ! -s "$archive" ]; then
  echo "missing source archive: $archive" >&2
  exit 1
fi
case "$url" in
  https://github.com/berniemackie97/build-bouncer/releases/download/*|file:///*) ;;
  *) echo "unsupported source URL" >&2; exit 1 ;;
esac
if [[ "$url" == *\"* || "$url" == *$'\n'* || "$url" == *\\* ]]; then
  echo "invalid source URL" >&2
  exit 1
fi

checksum="$(shasum -a 256 "$archive" | cut -d' ' -f1)"
mkdir -p "$(dirname "$output")"
cat > "$output" <<RUBY
class BuildBouncer < Formula
  desc "Run project quality checks before Git pushes"
  homepage "https://github.com/berniemackie97/build-bouncer"
  url "$url"
  sha256 "$checksum"
  license "MIT"
  version "$version"

  depends_on "go" => :build
  deny_network_access!

  def fetch
    system "go", "mod", "download"
  end

  def install
    system "go", "build", *std_go_args(
      output: bin/"build-bouncer",
      ldflags: "-X main.version=v#{version} -X main.commit=$commit"
    ), "./cmd/build-bouncer"
  end

  test do
    assert_equal "build-bouncer v#{version} ($commit)", shell_output("#{bin}/build-bouncer --version").strip
    assert_match "Usage:", shell_output("#{bin}/build-bouncer --help")
  end
end
RUBY
