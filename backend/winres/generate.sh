#!/usr/bin/env sh
# Generate the Windows VERSIONINFO resource object for the Go server.
#
# Called by the Makefile's backend-winres target on Windows. Requires
# MinGW-w64's windres; set WINDRES=/path/to/windres when it is not on PATH.
#
# Inputs:  VERSION (semver like v0.1.0, or dev), GOOS (windows or other), GOARCH (amd64)
# Outputs: backend/winres/version.h
#          backend/rsrc_windows_<GOARCH>.syso
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
backend_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)
version=${VERSION:-dev}
goos=${GOOS:-windows}
goarch=${GOARCH:-amd64}
windres=${WINDRES:-windres}
out="$backend_dir/rsrc_windows_${goarch}.syso"

if [ "$goos" != "windows" ]; then
  echo "winres: GOOS=$goos, skip Windows VERSIONINFO"
  exit 0
fi

case "$goarch" in
  amd64) target=pe-x86-64 ;;
  *)
    echo "winres: unsupported GOARCH=$goarch (only amd64 is built)" >&2
    exit 1
    ;;
esac

if ! command -v "$windres" >/dev/null 2>&1; then
  echo "winres: windres not found: $windres" >&2
  echo "        安装 MinGW-w64，或设置 WINDRES=/path/to/windres" >&2
  exit 1
fi

# v0.1.0 / 0.1.0 / 0.1.0-rc1 -> 0 1 0; dev or other non-numeric forms -> 0 0 0.
clean=$(printf '%s' "$version" | sed -E 's/^v//; s/[-+].*$//')
major=$(printf '%s' "$clean" | cut -d. -f1)
minor=$(printf '%s' "$clean" | cut -d. -f2)
patch=$(printf '%s' "$clean" | cut -d. -f3)
case "$major" in ''|*[!0-9]*) major=0 ;; esac
case "$minor" in ''|*[!0-9]*) minor=0 ;; esac
case "$patch" in ''|*[!0-9]*) patch=0 ;; esac

if [ "$major$minor$patch" = "000" ]; then
  # Keep the actual dev label in the string fields instead of pretending it is 0.0.0.0.
  display=$(printf '%s' "$clean" | sed 's/["\\]/ /g')
  [ -n "$display" ] || display=dev
else
  display="$major.$minor.$patch.0"
fi
comment=$(printf '%s' "$version" | sed 's/["\\]/ /g')
[ -n "$comment" ] || comment=dev

{
  printf '#define VERSION_MAJOR %s\n' "$major"
  printf '#define VERSION_MINOR %s\n' "$minor"
  printf '#define VERSION_PATCH %s\n' "$patch"
  printf '#define VERSION_BUILD 0\n'
  printf '#define VERSION_STR "%s"\n' "$display"
  printf '#define VERSION_COMMENT "version=%s"\n' "$comment"
} > "$script_dir/version.h"

# windres is a native Windows program under Git Bash; cygpath keeps -I/-o and
# the .rc path from being mangled before MSYS path conversion.
to_native() {
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -w "$1"
  else
    printf '%s\n' "$1"
  fi
}

"$windres" -I "$(to_native "$script_dir")" -O coff -F "$target" \
  "$(to_native "$script_dir/ebook-server.rc")" -o "$(to_native "$out")"

printf 'winres: %s -> %s (FileVersion %s)\n' \
  "$script_dir/ebook-server.rc" "$out" "$display"
