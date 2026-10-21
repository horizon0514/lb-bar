#!/usr/bin/env bash
# package.sh OS...: turns what `mygo build` left in build/ into the release
# files in build/dist/, for each OS given (macos, windows, linux; all
# three by default):
#   LB-Bar-<v>-macos-universal.zip             the app, signed ad hoc as dev.horizon.lbbar
#   LB-Bar-<v>-windows-<arch>-setup.exe        the NSIS installer
#   LB-Bar-<v>-windows-<arch>-portable.zip     the app's folder
#   lb-bar_<v>_<arch>.deb, lb-bar-<v>-linux-<arch>.tar.gz
set -euo pipefail
export PYTHONUTF8=1
cd "$(dirname "$0")/.."
# python3 may be a stub that only opens the Store on Windows: the one that runs.
py=
for c in python3 python; do
  if command -v "$c" >/dev/null 2>&1 && "$c" -c 'import zipfile' >/dev/null 2>&1; then py=$c; break; fi
done
[ -n "$py" ] || { echo "no python" >&2; exit 1; }
v=$("$py" -c 'import json;print(json.load(open("mygo.json",encoding="utf-8"))["version"])')
dist=build/dist
mkdir -p "$dist"
[ $# -gt 0 ] || set -- macos windows linux

for os in "$@"; do
  case "$os" in
  macos)
    app="build/darwin-universal/LB Bar.app"
    [ -d "$app" ] || { echo "no $app" >&2; exit 1; }
    sh scripts/sign.sh "$app"
    out="$dist/LB-Bar-$v-macos-universal.zip"
    rm -f "$out"
    if command -v ditto >/dev/null 2>&1; then
      ditto -c -k --sequesterRsrc --keepParent "$app" "$out"
    else
      "$py" scripts/zipdir.py "$out" build/darwin-universal "LB Bar.app"
    fi
    ;;
  windows)
    for a in amd64 arm64; do
      d="build/windows-$a"
      [ -d "$d" ] || continue
      setup="$d/LB Bar Setup $v $a.exe"
      if [ -f "$setup" ]; then cp "$setup" "$dist/LB-Bar-$v-windows-$a-setup.exe"; else echo "note: no installer for $a (NSIS missing?)" >&2; fi
      # The portable folder: the app and its resources, not the installer.
      stage=$(mktemp -d)
      mkdir "$stage/LB Bar"
      for f in "$d"/*; do
        case "$(basename "$f")" in *Setup*.exe) ;; *) cp -R "$f" "$stage/LB Bar/" ;; esac
      done
      rm -f "$dist/LB-Bar-$v-windows-$a-portable.zip"
      "$py" scripts/zipdir.py "$dist/LB-Bar-$v-windows-$a-portable.zip" "$stage" "LB Bar"
      rm -rf "$stage"
    done
    ;;
  linux)
    for a in amd64 arm64; do
      d="build/linux-$a"
      [ -d "$d" ] || continue
      cp "$d/lb-bar_${v}_$a.deb" "$d/lb-bar-$v-linux-$a.tar.gz" "$dist/"
    done
    ;;
  *) echo "unknown OS $os" >&2; exit 2 ;;
  esac
done
ls -l "$dist"
