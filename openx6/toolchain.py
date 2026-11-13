"""Locate the offline toolchain used by full OpenX6 builds."""
import os
import platform
import shutil
from pathlib import Path


def compiler_tools():
    def locate(name, candidates):
        for item in candidates:
            if item and Path(item).is_file() and os.access(item, os.X_OK):
                return str(item)
        raise ValueError('Missing tool: ' + name)
    bundled = os.getenv('OPENX6_TOOL_BIN')
    if bundled:
        directory = Path(bundled)
        require_tools = {name: locate(name, [directory/name]) for name in ('clang','llvm-strip','zig','lld')}
        return {k:require_tools[k] for k in ('clang','zig','lld')} | {'go':locate('go',[directory.parent/'go/bin/go'])}
    clang = locate('LLVM clang and llvm-strip', [p for p in
        [shutil.which('clang'), '/opt/homebrew/opt/llvm/bin/clang','/usr/local/opt/llvm/bin/clang']
        if p and Path(p).resolve().with_name('llvm-strip').is_file()])
    rust_host = ('aarch64' if platform.machine()=='arm64' else platform.machine())+'-apple-darwin'
    rust_lld = Path.home()/'.rustup/toolchains'/('stable-'+rust_host)/'lib/rustlib'/rust_host/'bin/rust-lld'
    lld = locate('GNU-flavor LLD', [os.getenv('OPENX6_LLD'), shutil.which('ld.lld'), shutil.which('lld'),
        '/opt/homebrew/opt/lld/bin/lld', '/usr/local/opt/lld/bin/lld', rust_lld])
    return dict(go=locate('go',[shutil.which('go')]), clang=clang, lld=lld, zig=locate('zig',[shutil.which('zig')]))
