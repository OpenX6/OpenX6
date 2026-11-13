#!/bin/sh
set -eu
export PATH=/opt/toolchain/bin:/opt/toolchain/go/bin:/usr/bin:/usr/sbin:/bin
export GOROOT=/opt/toolchain/go GOTOOLCHAIN=local GOPROXY=off GOENV=off GOSUMDB=off
export ZIG_LIB_DIR=/opt/toolchain/zig-lib OPENX6_LLD=/opt/toolchain/bin/lld
exec /usr/bin/python3 -IB /opt/openx6/desktop-runtime/windows/bridge.py "$@"
