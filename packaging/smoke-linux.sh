#!/bin/sh
# Only run in the disposable native CI container, as root.
set -eu
format=$1
case "$format" in
 deb) dpkg -i build/packages/*.deb ;;
 rpm) dnf install -y build/packages/*.rpm ;;
 *) exit 1 ;;
esac
ldd /usr/lib/idm-enhanced/desktop
if ldd /usr/lib/idm-enhanced/desktop | grep -q 'not found'; then exit 1; fi
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
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
echo 'PASS package install, linked libraries, user host registration, unregister, uninstall and data retention'
