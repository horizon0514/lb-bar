#!/usr/bin/env bash
# screenshot-linux.sh BINARY OUTDIR: runs the app on the example data
# (LBBAR_DEMO=1) in a virtual X display with a window manager, and saves
# screenshots of the list, the detail and the cheat sheet. Needs xvfb,
# openbox, xdotool, imagemagick and dbus-x11. CI runs it; so can you.
set -euo pipefail
bin=$(realpath "$1"); out=$(realpath -m "$2"); mkdir -p "$out"
if [ "${3:-}" != --inside ]; then
  exec xvfb-run -a -s "-screen 0 1440x900x24" dbus-run-session -- bash "$0" "$bin" "$out" --inside
fi
{
  export HOME=$(mktemp -d) LBBAR_DEMO=1
  export XDG_CONFIG_HOME="$HOME/.config"
  openbox >/dev/null 2>&1 &
  sleep 1; xsetroot -solid '#2f4f6f' || true
  "$bin" >"$out/app.log" 2>&1 &
  pid=$!
  sleep 12
  w=$(xdotool search --onlyvisible --name "LB Bar" | head -1 || true)
  import -window root "$out/linux-list.png"
  if [ -n "$w" ]; then
    xdotool windowactivate --sync "$w" || true
    sleep 1; xdotool key Return; sleep 4
    import -window root "$out/linux-detail.png"
    xdotool key Escape; sleep 1; xdotool key ctrl+slash; sleep 2
    import -window root "$out/linux-cheatsheet.png"
  else
    echo "no visible LB Bar window" >>"$out/app.log"
  fi
  kill "$pid" 2>/dev/null || true
}
ls -l "$out"
