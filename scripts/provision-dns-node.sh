#!/bin/sh
# Provision (or re-converge) one Portico DNS node with native services only:
# dnsdist (PowerDNS dnsdist-20 repo) on public :53, portico-dns under its own
# user, BIND 9 as the acme.getportico.tv authority on WireGuard, nftables.
# No Docker. See deployment/hosted/TOPOLOGY.md.
#
#   scripts/provision-dns-node.sh ns1     # portico-dns-primary-1, ACME primary
#   scripts/provision-dns-node.sh ns2     # portico-dns-de-1, ACME secondary
#
# Prerequisites on the node (never copied by this script; secrets stay there):
#   /etc/wireguard/wg0.conf               wg0 up, peers = Hosted + the other node
#   /etc/bind/portico-keys.conf           root:bind 0640; key "acme-transfer."
#                                         (both nodes) and "acme-update." (ns1)
#   /etc/portico-dns/feed-secret          portico-dns 0600 (Hosted's DNS feed key)
# The portico-dns binary is installed by scripts/deploy-dns-release.py.
set -eu

HOSTED_PUBLIC_IP=159.195.14.223
HOSTED_WG_IP=10.77.0.10
case "${1:-}" in
  ns1) SSH_HOST=portico-dns-primary-1; ROLE=primary;   PUBLIC_IP=174.138.183.45; WG_IP=10.77.0.2; PEER_PUBLIC_IP=188.68.32.46;   PEER_WG_IP=10.77.0.3 ;;
  ns2) SSH_HOST=portico-dns-de-1;      ROLE=secondary; PUBLIC_IP=188.68.32.46;   WG_IP=10.77.0.3; PEER_PUBLIC_IP=174.138.183.45; PEER_WG_IP=10.77.0.2 ;;
  *) echo "usage: $0 ns1|ns2" >&2; exit 2 ;;
esac
PUBLIC_IF=eth0
ROOT=$(cd "$(dirname "$0")/.." && pwd)
SRC="$ROOT/deployment/dns"

render() {
  sed -e "s/@PUBLIC_IP@/$PUBLIC_IP/g" -e "s/@WG_IP@/$WG_IP/g" -e "s/@PEER_WG_IP@/$PEER_WG_IP/g" \
      -e "s/@PEER_PUBLIC_IP@/$PEER_PUBLIC_IP/g" -e "s/@HOSTED_PUBLIC_IP@/$HOSTED_PUBLIC_IP/g" \
      -e "s/@HOSTED_WG_IP@/$HOSTED_WG_IP/g" -e "s/@PUBLIC_IF@/$PUBLIC_IF/g" "$1"
}

STAGE=$(mktemp -d)
trap 'rm -rf "${STAGE:?}"' EXIT
render "$SRC/dnsdist.conf.in" > "$STAGE/dnsdist.conf"
render "$SRC/named.$ROLE.conf.in" > "$STAGE/named.conf"
render "$SRC/nftables.conf.in" > "$STAGE/nftables.conf"
cp "$SRC/portico-dns.service" "$SRC/portico-dns.env" "$STAGE/"

ssh "$SSH_HOST" 'rm -rf /root/portico-dns-provision && mkdir -m 0700 /root/portico-dns-provision'
scp -q "$STAGE"/* "$SSH_HOST:/root/portico-dns-provision/"

ssh "$SSH_HOST" "PUBLIC_IP=$PUBLIC_IP WG_IP=$WG_IP sh -s" <<'REMOTE'
set -eu
export DEBIAN_FRONTEND=noninteractive
P=/root/portico-dns-provision
[ -s /etc/bind/portico-keys.conf ] || { echo "missing /etc/bind/portico-keys.conf" >&2; exit 1; }
ip -4 addr show wg0 | grep -q "inet $WG_IP/" || { echo "wg0 is not up with $WG_IP" >&2; exit 1; }

. /etc/os-release
case "$ID" in debian) repo="https://repo.powerdns.com/debian $VERSION_CODENAME-dnsdist-20 main" ;;
              ubuntu) repo="https://repo.powerdns.com/ubuntu $VERSION_CODENAME-dnsdist-20 main" ;;
              *) echo "unsupported OS $ID" >&2; exit 1 ;; esac
# Packages must not start with their default configs (a default named would
# listen on :53): hold service starts until the configs below are in place.
printf '#!/bin/sh\nexit 101\n' > /usr/sbin/policy-rc.d; chmod 0755 /usr/sbin/policy-rc.d
trap 'rm -f /usr/sbin/policy-rc.d' EXIT
apt-get update -qq
apt-get install -y -qq curl gnupg nftables wireguard-tools bind9 bind9-dnsutils >/dev/null
curl -fsSL https://repo.powerdns.com/FD380FBB-pub.asc | gpg --batch --yes --dearmor -o /usr/share/keyrings/powerdns.gpg
echo "deb [signed-by=/usr/share/keyrings/powerdns.gpg arch=amd64] $repo" > /etc/apt/sources.list.d/powerdns-dnsdist.list
printf 'Package: dnsdist*\nPin: origin repo.powerdns.com\nPin-Priority: 600\n' > /etc/apt/preferences.d/dnsdist
apt-get update -qq
apt-get install -y -qq dnsdist >/dev/null

# portico-dns: own user, config, unit
getent group portico-dns >/dev/null || groupadd --system portico-dns
getent passwd portico-dns >/dev/null || useradd --system --gid portico-dns --home-dir /nonexistent --no-create-home --shell /usr/sbin/nologin portico-dns
install -d -o root -g portico-dns -m 0750 /etc/portico-dns
install -o root -g portico-dns -m 0640 $P/portico-dns.env /etc/portico-dns/portico-dns.env
[ -f /etc/portico-dns/feed-secret ] && chown portico-dns:portico-dns /etc/portico-dns/feed-secret && chmod 0600 /etc/portico-dns/feed-secret
install -d -o root -g root -m 0755 /opt/portico-dns /opt/portico-dns/releases
install -o root -g root -m 0644 $P/portico-dns.service /etc/systemd/system/portico-dns.service

# BIND: WireGuard-only authority for acme.getportico.tv
chown root:bind /etc/bind; chmod 2755 /etc/bind
install -o root -g bind -m 0640 $P/named.conf /etc/bind/named.conf
chown root:bind /etc/bind/portico-keys.conf; chmod 0640 /etc/bind/portico-keys.conf
install -d /etc/systemd/system/named.service.d
printf '[Unit]\nAfter=wg-quick@wg0.service\nWants=wg-quick@wg0.service\n' > /etc/systemd/system/named.service.d/portico.conf
named-checkconf /etc/bind/named.conf

# dnsdist
install -o root -g _dnsdist -m 0640 $P/dnsdist.conf /etc/dnsdist/dnsdist.conf
dnsdist --check-config -C /etc/dnsdist/dnsdist.conf >/dev/null

# firewall
nft -c -f $P/nftables.conf
install -o root -g root -m 0755 $P/nftables.conf /etc/nftables.conf

systemctl daemon-reload
systemctl enable wg-quick@wg0 nftables named dnsdist >/dev/null 2>&1
if [ -x /opt/portico-dns/current/portico-dns ]; then systemctl enable portico-dns >/dev/null 2>&1; fi
rm -rf "${P:?}"
echo "provisioned: $(hostname) dnsdist=$(dpkg-query -W -f='${Version}' dnsdist) bind9=$(dpkg-query -W -f='${Version}' bind9)"
REMOTE
echo "Configuration installed. Services are enabled but were not restarted; see TOPOLOGY.md for the cut-over order."
