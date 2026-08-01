#!/bin/sh
set -eu

ROOT=${1:-.}
BACKUP=$(mktemp -d /tmp/meowdeck-install.XXXXXX)
SUCCESS=0
WAS_RUNNING=0
WAS_ENABLED=0

[ "$(id -u)" -eq 0 ] || { echo "run as root on OpenWrt" >&2; exit 1; }
[ -x "$ROOT/meowdeck" ] || { echo "missing release binary: $ROOT/meowdeck" >&2; exit 1; }
LAN_IP=$(uci -q get network.lan.ipaddr || true)
[ -n "$LAN_IP" ] || { echo "cannot detect the OpenWrt LAN address" >&2; exit 1; }

backup_file() {
	target=$1
	name=$2
	if [ -e "$target" ]; then
		cp -p "$target" "$BACKUP/$name"
		: >"$BACKUP/$name.exists"
	fi
}

restore_file() {
	target=$1
	name=$2
	if [ -f "$BACKUP/$name.exists" ]; then
		cp -p "$BACKUP/$name" "$target"
	else
		rm -f "$target"
	fi
}

rollback() {
	status=$?
	trap - EXIT HUP INT TERM
	if [ "$SUCCESS" -eq 1 ]; then
		rm -rf "$BACKUP"
		return
	fi
	set +e
	echo "installation failed; restoring previous configuration" >&2
	/etc/init.d/meowdeck stop >/dev/null 2>&1
	restore_file /usr/bin/meowdeck meowdeck.bin
	restore_file /etc/meowdeck/config.json config.json
	restore_file /etc/init.d/meowdeck meowdeck.init
	restore_file /usr/bin/meowdeck-update meowdeck-update
	restore_file /usr/bin/meowdeck-configure meowdeck-configure
	restore_file /etc/meowdeck/update.conf update.conf
	restore_file /etc/nginx/conf.d/meowdeck.conf meowdeck.nginx.conf
	restore_file /usr/share/meowdeck/nginx.conf.template nginx.conf.template
	restore_file /usr/share/meowdeck/dnsmasq.uci.template dnsmasq.uci.template
	if [ "$WAS_ENABLED" -eq 1 ] && [ -x /etc/init.d/meowdeck ]; then
		/etc/init.d/meowdeck enable >/dev/null 2>&1
	elif [ -x /etc/init.d/meowdeck ]; then
		/etc/init.d/meowdeck disable >/dev/null 2>&1
	fi
	uci import dhcp <"$BACKUP/dhcp.uci"
	uci commit dhcp
	/etc/init.d/dnsmasq restart >/dev/null 2>&1
	nginx -s reload >/dev/null 2>&1
	if [ "$WAS_RUNNING" -eq 1 ] && [ -x /etc/init.d/meowdeck ]; then
		/etc/init.d/meowdeck start >/dev/null 2>&1
	fi
	rm -rf "$BACKUP"
	exit "$status"
}

[ -x /etc/init.d/meowdeck ] && /etc/init.d/meowdeck running >/dev/null 2>&1 && WAS_RUNNING=1
ls /etc/rc.d/S*meowdeck >/dev/null 2>&1 && WAS_ENABLED=1
backup_file /usr/bin/meowdeck meowdeck.bin
backup_file /etc/meowdeck/config.json config.json
backup_file /etc/init.d/meowdeck meowdeck.init
backup_file /usr/bin/meowdeck-update meowdeck-update
backup_file /usr/bin/meowdeck-configure meowdeck-configure
backup_file /etc/meowdeck/update.conf update.conf
backup_file /etc/nginx/conf.d/meowdeck.conf meowdeck.nginx.conf
backup_file /usr/share/meowdeck/nginx.conf.template nginx.conf.template
backup_file /usr/share/meowdeck/dnsmasq.uci.template dnsmasq.uci.template
uci export dhcp >"$BACKUP/dhcp.uci"
trap rollback EXIT HUP INT TERM

mkdir -p /etc/meowdeck /etc/nginx/conf.d /usr/share/meowdeck
cp "$ROOT/meowdeck" /usr/bin/meowdeck
chmod 0755 /usr/bin/meowdeck

if [ ! -f /etc/meowdeck/config.json ]; then
	sed "s/192\.168\.8\.1/$LAN_IP/g" "$ROOT/config.example.json" >/etc/meowdeck/config.json
	chmod 0600 /etc/meowdeck/config.json
fi

cp "$ROOT/meowdeck.init" /etc/init.d/meowdeck
chmod 0755 /etc/init.d/meowdeck
cp "$ROOT/meowdeck-update" /usr/bin/meowdeck-update
chmod 0755 /usr/bin/meowdeck-update
cp "$ROOT/meowdeck-configure" /usr/bin/meowdeck-configure
chmod 0755 /usr/bin/meowdeck-configure
if [ ! -f /etc/meowdeck/update.conf ]; then
	cp "$ROOT/update.conf" /etc/meowdeck/update.conf
fi
cp "$ROOT/meowdeck.nginx.conf" /usr/share/meowdeck/nginx.conf.template
cp "$ROOT/meowdeck.dnsmasq.uci" /usr/share/meowdeck/dnsmasq.uci.template

/usr/bin/meowdeck-configure --no-restart
/etc/init.d/meowdeck enable
/etc/init.d/meowdeck restart

healthy=0
for _ in 1 2 3 4 5; do
	if curl -fsS http://127.0.0.1:9080/healthz >/dev/null 2>&1; then
		healthy=1
		break
	fi
	sleep 1
done
[ "$healthy" -eq 1 ] || { echo "MeowDeck health check failed" >&2; exit 1; }

SUCCESS=1
HOSTNAME=$(/usr/bin/meowdeck -config /etc/meowdeck/config.json -print-hostname)
echo "MeowDeck installed: http://$HOSTNAME"
