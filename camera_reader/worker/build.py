"""Compile the ARM64 camera helpers for inclusion in a desktop distribution."""
import subprocess
from pathlib import Path


def build(destination):
    source = Path(__file__).resolve().parent
    destination.mkdir(parents=True, exist_ok=True)
    subprocess.run(['zig', 'cc', '-target', 'aarch64-linux-musl', '-Os', '-s', '-static',
                    source/'fusein.c', '-o', destination/'fusein'], check=True)
    subprocess.run(['zig', 'cc', '-target', 'aarch64-linux-gnu.2.35', '-O2', '-s', '-shared',
                    '-fPIC', source/'caller.c', '-o', destination/'caller.so', '-ldl'], check=True)
    (destination/'profile.py').write_bytes((source/'profile.py').read_bytes())
    (destination/'guest.py').write_bytes((source/'guest.py').read_bytes())
    (destination/'deploy.py').write_bytes((source/'deploy.py').read_bytes())
