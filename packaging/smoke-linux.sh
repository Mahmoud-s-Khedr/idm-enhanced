#!/bin/sh
# Only run in the disposable native CI container, as root.
set -eu
format=$1
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
# A controlled lower-version package tests native upgrade mechanics. Released
# schema upgrades are covered separately by deterministic storage fixtures.
case "$format" in
 deb) dpkg -i build/predecessor/*.deb ;;
 rpm) dnf install -y build/predecessor/*.rpm ;;
 *) exit 1 ;;
esac
/usr/lib/idm-enhanced/downloader --state-dir "$scratch/state" enqueue -o "$scratch/retained.bin" https://example.invalid/retained
before=$(/usr/lib/idm-enhanced/downloader --state-dir "$scratch/state" status)
case "$format" in
 deb) dpkg -i build/packages/*.deb ;;
 rpm) dnf install -y build/packages/*.rpm ;;
esac
after=$(/usr/lib/idm-enhanced/downloader --state-dir "$scratch/state" status)
test "$before" = "$after"
ldd /usr/lib/idm-enhanced/desktop
if ldd /usr/lib/idm-enhanced/desktop | grep -q 'not found'; then exit 1; fi
export HOME="$scratch" XDG_CONFIG_HOME="$scratch/config"
mkdir -p "$XDG_CONFIG_HOME/idm-enhanced"
printf 'preserve\n' > "$XDG_CONFIG_HOME/idm-enhanced/user-data"
/usr/lib/idm-enhanced/native-host --register
/usr/lib/idm-enhanced/native-host --register
test -f "$HOME/.mozilla/native-messaging-hosts/com.idmenhanced.desktop.json"
test -f "$XDG_CONFIG_HOME/google-chrome/NativeMessagingHosts/com.idmenhanced.desktop.json"
/usr/lib/idm-enhanced/native-host --unregister
test ! -f "$HOME/.mozilla/native-messaging-hosts/com.idmenhanced.desktop.json"
case "$format" in
 deb) dpkg -r idm-enhanced ;;
 rpm) dnf remove -y idm-enhanced ;;
esac
test ! -f /usr/lib/idm-enhanced/desktop
test -f "$XDG_CONFIG_HOME/idm-enhanced/user-data"
test -f "$scratch/state/jobs.db"
echo 'PASS package install, controlled version upgrade with durable queue retention, linked libraries, user host registration, unregister, uninstall and data retention'
