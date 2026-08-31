#!/bin/sh
set -eu

contents_dir="$(cd -P -- "$(dirname -- "$0")/.." && pwd)"
resources_dir="$contents_dir/Resources"
app_data_dir="$HOME/Library/Application Support/Portico Media Server"
agents_dir="$HOME/Library/LaunchAgents"
domain="gui/$(id -u)"
bundle_build="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' "$contents_dir/Info.plist")"

mkdir -p "$app_data_dir/logs" "$agents_dir"

install_agent() {
  label="$1"
  template="$2"
  destination="$agents_dir/$label.plist"
  temporary="$(mktemp "$agents_dir/.portico-agent.XXXXXX")"
  cp "$template" "$temporary"

  if [ "$label" = "tv.getportico.server.service" ]; then
    /usr/libexec/PlistBuddy -c "Set :ProgramArguments:0 $resources_dir/bin/portico-media-server" "$temporary"
    /usr/libexec/PlistBuddy -c "Set :EnvironmentVariables:PORTICO_APP_DATA $app_data_dir" "$temporary"
    /usr/libexec/PlistBuddy -c "Set :EnvironmentVariables:PORTICO_WEB_DIST $resources_dir/web" "$temporary"
    /usr/libexec/PlistBuddy -c "Set :EnvironmentVariables:PORTICO_FFMPEG_PATH $resources_dir/bin/ffmpeg" "$temporary"
    /usr/libexec/PlistBuddy -c "Set :EnvironmentVariables:PORTICO_FFPROBE_PATH $resources_dir/bin/ffprobe" "$temporary"
	/usr/libexec/PlistBuddy -c "Set :EnvironmentVariables:PORTICO_INSTALL_BUILD $bundle_build" "$temporary"
    /usr/libexec/PlistBuddy -c "Set :StandardOutPath $app_data_dir/logs/launchd-server.log" "$temporary"
    /usr/libexec/PlistBuddy -c "Set :StandardErrorPath $app_data_dir/logs/launchd-server-error.log" "$temporary"
  else
    /usr/libexec/PlistBuddy -c "Set :ProgramArguments:0 $resources_dir/bin/portico-desktop" "$temporary"
	/usr/libexec/PlistBuddy -c "Set :EnvironmentVariables:PORTICO_INSTALL_BUILD $bundle_build" "$temporary"
    /usr/libexec/PlistBuddy -c "Set :StandardOutPath $app_data_dir/logs/companion.log" "$temporary"
    /usr/libexec/PlistBuddy -c "Set :StandardErrorPath $app_data_dir/logs/companion-error.log" "$temporary"
  fi

  if [ ! -f "$destination" ] || ! cmp -s "$temporary" "$destination"; then
	program="$(/usr/libexec/PlistBuddy -c 'Print :ProgramArguments:0' "$temporary")"
    launchctl bootout "$domain/$label" >/dev/null 2>&1 || true
	shutdown_attempt=0
	while pgrep -f "$program" >/dev/null 2>&1; do
	  shutdown_attempt=$((shutdown_attempt + 1))
	  if [ "$shutdown_attempt" -ge 15 ]; then
		echo "Portico could not stop the previous $label process" >&2
		exit 1
	  fi
	  sleep 1
	done
    install -m 600 "$temporary" "$destination"
  fi
  rm -f "$temporary"
  launchctl enable "$domain/$label" >/dev/null 2>&1 || true
  if ! launchctl print "$domain/$label" >/dev/null 2>&1; then
	attempt=0
	while ! launchctl bootstrap "$domain" "$destination" >/dev/null 2>&1; do
	  attempt=$((attempt + 1))
	  if [ "$attempt" -ge 5 ]; then
		echo "Portico could not start $label after five attempts" >&2
		exit 1
	  fi
	  sleep 1
	done
  fi
  launchctl kickstart "$domain/$label" >/dev/null 2>&1 || true
}

install_agent "tv.getportico.server.service" "$resources_dir/launchd/tv.getportico.server.service.plist"
install_agent "tv.getportico.server.companion" "$resources_dir/launchd/tv.getportico.server.companion.plist"
