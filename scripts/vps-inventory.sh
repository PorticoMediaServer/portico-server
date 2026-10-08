#!/bin/sh
# Read-only inventory of one Portico VPS, for comparison with
# deployment/hosted/TOPOLOGY.md. Prints no secrets: file contents are never
# read, only names, owners and modes.
#
#   ssh portico-core-us-1 'sh -s' < scripts/vps-inventory.sh
#
# It changes nothing on the host.
set -u
LC_ALL=C
export LC_ALL

section() { printf '\n## %s\n' "$1"; }

section "host"
printf 'hostname: %s\n' "$(hostname)"
if [ -r /etc/os-release ]; then . /etc/os-release; printf 'os: %s\n' "${PRETTY_NAME:-unknown}"; fi
printf 'kernel: %s\n' "$(uname -r)"
ip -br address 2>/dev/null | awk '$1 != "lo" {print "address: " $0}'

section "enabled or running units (non-distribution)"
systemctl list-unit-files --type=service,timer,socket,path --state=enabled --no-legend --no-pager --plain 2>/dev/null \
  | awk '{print $1}' | sort > /tmp/.inv-enabled.$$
systemctl list-units --type=service,timer,socket --state=running,active --no-legend --no-pager --plain 2>/dev/null \
  | awk '{print $1}' | sort > /tmp/.inv-running.$$
sort -u /tmp/.inv-enabled.$$ /tmp/.inv-running.$$ \
  | grep -v -E '^(systemd-|dbus|getty@|serial-getty@|user@|user-runtime-dir@|console-|keyboard-setup|kmod-|modprobe@|setvtrgb|e2scrub|fstrim|apt-daily|dpkg-db-backup|logrotate|man-db|motd-news|ua-|ubuntu-|update-notifier|snapd|ssh|sshd|cron|rsyslog|polkit|udisks2|networkd-dispatcher|systemd|getty|remote-|blk-availability|lvm2|dm-event|multipathd|open-iscsi|iscsid|cloud-|finalrd|grub-|lxd|secureboot|apparmor|unattended-upgrades|qemu-guest-agent|chrony|wpa_supplicant|upower|fwupd|sysstat|pollinate|console-setup|nftables|wg-quick@|networking|ifup@|emergency|rescue|plymouth|dmesg|thermald|irqbalance|vgauth|open-vm-tools|ModemManager|rsync|uuidd|sysstat-)' \
  | while read -r unit; do
      state=$(systemctl is-active "$unit" 2>/dev/null)
      enabled=$(systemctl is-enabled "$unit" 2>/dev/null)
      printf '%-44s %-9s %s\n' "$unit" "$state" "$enabled"
    done
rm -f /tmp/.inv-enabled.$$ /tmp/.inv-running.$$
printf 'infrastructure: '
for u in nftables.service wg-quick@wg0.service qemu-guest-agent.service ssh.service; do
  printf '%s=%s ' "$u" "$(systemctl is-active "$u" 2>/dev/null)"
done
printf '\n'

section "unit files under /etc/systemd/system (excluding .wants links)"
find /etc/systemd/system -maxdepth 2 \( -type f -o -type l \) ! -path '*/*.wants/*' ! -path '*/*.requires/*' 2>/dev/null \
  | grep -v -E '/(dbus-org\.|sshd?\.service|syslog\.service|snap-|snapd\.|cloud-init|iscsi|display-manager|vmtoolsd)' | sort

section "timers"
systemctl list-timers --all --no-legend --no-pager 2>/dev/null | awk '{for (i=1;i<=NF;i++) if ($i ~ /\.timer$/) {print $i; break}}' | sort

section "cron"
for f in /etc/crontab /etc/cron.d/*; do [ -f "$f" ] && grep -H -v -E '^\s*(#|$|SHELL=|PATH=|MAILTO=)' "$f"; done 2>/dev/null
for d in /var/spool/cron/crontabs /var/spool/cron; do
  [ -d "$d" ] && find "$d" -maxdepth 1 -type f 2>/dev/null | while read -r f; do grep -H -v -E '^\s*(#|$)' "$f"; done
done
printf 'cron.hourly/daily/weekly/monthly: '
ls /etc/cron.hourly /etc/cron.daily /etc/cron.weekly /etc/cron.monthly 2>/dev/null | grep -v -E '^(/|$|\.placeholder)' | tr '\n' ' '
printf '\n'

section "listening sockets"
ss -H -ltnu 2>/dev/null | awk '{print $1, $5}' | sort -u
ss -H -ltnup 2>/dev/null | sed -n 's/.*users:(("\([^"]*\)".*/\1/p' | sort -u | tr '\n' ' '
printf '\n'

section "firewall (nftables)"
nft list ruleset 2>/dev/null | grep -v -E '^\s*$' || echo "nft unavailable"

section "wireguard"
if command -v wg >/dev/null 2>&1; then wg show all 2>/dev/null | grep -E '^(interface|peer|  endpoint|  allowed ips|  listening port)' ; fi

section "containers"
if command -v docker >/dev/null 2>&1; then docker ps -a --format '{{.Names}} {{.Image}} {{.Status}}' 2>/dev/null; else echo "docker: not installed"; fi
if command -v containerd >/dev/null 2>&1; then echo "containerd: installed"; else echo "containerd: not installed"; fi

section "users (uid >= 100, with a shell or owning a service)"
awk -F: '$3 >= 100 && $3 < 65534 {print $1, $3, $6, $7}' /etc/passwd

section "ssh authorized_keys (type and comment only)"
printf 'sshd AuthorizedKeysFile: %s\n' "$(sshd -T 2>/dev/null | awk '$1 == "authorizedkeysfile" {$1=""; print substr($0, 2)}')"
for f in /root/.ssh/authorized_keys /root/.ssh/authorized_keys2 /home/*/.ssh/authorized_keys /home/*/.ssh/authorized_keys2 /etc/ssh/authorized_keys/*; do
  [ -f "$f" ] && awk -v f="$f" '!/^\s*(#|$)/ {c=""; for (i=3;i<=NF;i++) c=c" "$i; print f ": " $1 c}' "$f"
done

section "directories"
for d in /opt /srv /etc/portico* /var/lib/portico* /var/backups/portico* /etc/caddy /var/lib/caddy /etc/wireguard /etc/postgresql /etc/bind /etc/dnsdist; do
  [ -e "$d" ] || continue
  printf '%s\n' "$d"
  total=$(find "$d" -mindepth 1 -maxdepth 2 2>/dev/null | wc -l)
  find "$d" -mindepth 1 -maxdepth 2 2>/dev/null | sort | head -n 80 | while read -r p; do
    printf '  %s %s %s\n' "$(stat -c '%U:%G %a' "$p" 2>/dev/null)" "$( [ -d "$p" ] && echo dir || echo file)" "$p"
  done
  [ "$total" -gt 80 ] && printf '  ... %s more entries\n' "$((total - 80))"
done

section "packages of interest"
dpkg-query -W -f='${Package} ${Version}\n' 2>/dev/null \
  | grep -E '^(postgresql|caddy|docker|containerd|nginx|apache2|haproxy|redis|etcd|patroni|bind9|dnsdist|wireguard|socat|certbot|nodejs|python3-patroni|keepalived) ' || true

section "toolchains (build and test runner)"
for t in go node npm pnpm python3 git rsync gcc make psql; do
  if command -v "$t" >/dev/null 2>&1; then
    case "$t" in
      go) v=$(go version 2>/dev/null) ;;
      psql) v=$(psql --version 2>/dev/null) ;;
      *) v=$("$t" --version 2>/dev/null | head -n 1) ;;
    esac
    printf '%s: %s (%s)\n' "$t" "$v" "$(command -v "$t")"
  fi
done
