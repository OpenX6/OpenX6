#!/bin/sh
set -eu
cd "$(dirname "$0")"
export PATH="$PATH:/opt/homebrew/bin:/usr/local/bin"
for python in /usr/local/bin/python3 /opt/homebrew/bin/python3 python3; do
    if "$python" -c 'import sys,tkinter; assert sys.version_info >= (3,11)' 2>/dev/null; then
        exec "$python" -B -m openx6.desktop
    fi
done
echo 'OpenX6 requires Python 3.11+ with Tk.'
exit 1
