#!/bin/bash
set -eu
umask 077
kit=$(cd "$(dirname "$0")" && pwd)
export PATH=/opt/toolchain/bin:/opt/toolchain/go/bin:/usr/bin:/usr/sbin:/bin
export OPENX6_TOOL_BIN=/opt/toolchain/bin GOROOT=/opt/toolchain/go ZIG_LIB_DIR=/opt/toolchain/zig-lib
export PYTHONIOENCODING=utf-8 PYTHONDONTWRITEBYTECODE=1
export GOPROXY=off GOENV=off GOTOOLCHAIN=local GOSUMDB=off
test "$(id -un)" = openx6
if [ "$3" = gui ]; then
    if ! /usr/bin/python3 -c 'import tkinter; root=tkinter.Tk(); root.destroy()'; then
        echo 'WSLg and python3-tk are required for -Gui. See WINDOWS-TEST.md.' >&2
        exit 1
    fi
fi
(cd "$kit" && sha256sum -c SOURCE-SHA256SUMS)
base="$HOME/OpenX6-DevQA"
mkdir -p "$base"
run=$(mktemp -d "$base/run-XXXXXXXX")
mkdir "$run/source" "$run/output"
tar xzf "$kit/source.tar.gz" -C "$run/source"
cp -R "$kit/field_budget" "$run/"
cd "$run/source"
echo "Test directory: $run"
exec /usr/bin/python3 -B "$kit/acceptance.py" "$run" "$1" "$2" "$3"
