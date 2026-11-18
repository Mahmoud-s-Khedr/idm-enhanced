#!/bin/sh
# Disposable hosted Ubuntu runners only. Avoid slow regional EC2 mirrors while
# retaining apt's signed repository verification and the same native packages.
set -eu
for source in /etc/apt/sources.list /etc/apt/sources.list.d/ubuntu.sources; do
  if test -f "$source"; then
    sed -i -E 's#https?://[[:alnum:].-]*ec2\.archive\.ubuntu\.com/ubuntu/?#https://archive.ubuntu.com/ubuntu/#g' "$source"
  fi
done
apt-get -o Acquire::Retries=2 -o Acquire::http::Timeout=30 -o Acquire::https::Timeout=30 update
apt-get -o Acquire::Retries=2 -o Acquire::http::Timeout=30 -o Acquire::https::Timeout=30 install -y libgtk-4-dev libwebkitgtk-6.0-dev
