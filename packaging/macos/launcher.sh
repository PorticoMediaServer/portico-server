#!/bin/sh
# Opening Portico Media Server.app installs (or refreshes) a per-user launchd
# agent that runs the server in the background, then opens the web app.
set -eu

contents_dir="$(cd -P -- "$(dirname -- "$0")/.." && pwd)"
resources_dir="$contents_dir/Resources"
state_dir="$HOME/Library/Application Support/Portico Media Server"
agents_dir="$HOME/Library/LaunchAgents"
label="tv.getportico.server.service"
destination="$agents_dir/$label.plist"
domain="gui/$(id -u)"

mkdir -p "$state_dir/logs" "$agents_dir"
chmod 700 "$state_dir"
temporary="$(mktemp "$agents_dir/.portico-agent.XXXXXX")"
cp "$resources_dir/launchd/$label.plist" "$temporary"
/usr/libexec/PlistBuddy -c "Set :ProgramArguments:0 $resources_dir/server/portico-server" "$temporary"
/usr/libexec/PlistBuddy -c "Set :EnvironmentVariables:PORTICO_STATE_DIR $state_dir" "$temporary"
/usr/libexec/PlistBuddy -c "Set :EnvironmentVariables:PORTICO_WEB_DIR $resources_dir/server/web" "$temporary"
/usr/libexec/PlistBuddy -c "Set :StandardOutPath $state_dir/logs/server.log" "$temporary"
/usr/libexec/PlistBuddy -c "Set :StandardErrorPath $state_dir/logs/server-error.log" "$temporary"

if [ ! -f "$destination" ] || ! cmp -s "$temporary" "$destination"; then
  launchctl bootout "$domain/$label" >/dev/null 2>&1 || true
  install -m 600 "$temporary" "$destination"
fi
rm -f "$temporary"
launchctl enable "$domain/$label" >/dev/null 2>&1 || true
if ! launchctl print "$domain/$label" >/dev/null 2>&1; then
  attempt=0
  while ! launchctl bootstrap "$domain" "$destination" >/dev/null 2>&1; do
    attempt=$((attempt + 1))
    if [ "$attempt" -ge 5 ]; then
      echo "Portico could not start its background server after five attempts" >&2
      exit 1
    fi
    sleep 1
  done
fi
launchctl kickstart "$domain/$label" >/dev/null 2>&1 || true
sleep 2
open "http://127.0.0.1:32500/"
