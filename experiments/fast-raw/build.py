"""Build the experimental converter from the official library, with no device inputs."""
import argparse
import hashlib
from pathlib import Path
import shutil
import struct
import subprocess
import tempfile
import sys

HERE = Path(__file__).resolve().parent
ABI = HERE.parents[1] / 'components/src/pureshot/ps2native'
ISP_SHA = '12eefdd40aa2019b7efedc45264f9fbccd3b19324a01dc1258a54b4ddb468ec7'
# 1.2.36 snapshot / double / 120M / normal references the IMX01F double ATM
# descriptor. The ELF's data segment maps VA to file offset with delta 0x10000.
TUNE_OFFSET = 0x41e8c0 - 0x10000 + 0xecc - 96


def tuning(library):
    if hashlib.sha256(library).hexdigest() != ISP_SHA:
        raise ValueError('Requires the original X6 1.2.36 libinsispparam.so')
    return struct.unpack_from('<41f', library, TUNE_OFFSET)


def build(library, output):
    values = tuning(library.read_bytes())
    if output.exists():
        raise FileExistsError(output)
    with tempfile.TemporaryDirectory(dir=output.parent) as temp:
        tmp = Path(temp)
        (tmp/'stock_tuning.h').write_text('static const float raw_tune[] = {\n' +
            ',\n'.join(v.hex() + 'f' for v in values) + '\n};\n')
        binary = tmp/'ps2native'
        subprocess.run(['zig','cc','-target','aarch64-linux-gnu.2.35','-O2','-I',str(tmp),
                        '-I',str(ABI),str(HERE/'ps2native.c'),'-ldl','-lm','-o',str(binary)],check=True)
        strip = shutil.which('llvm-strip') or '/opt/homebrew/opt/llvm/bin/llvm-strip'
        subprocess.run([strip,'--strip-all',str(binary)],check=True)
        # Atomic publish without overwriting another invocation's output.
        output.hardlink_to(binary)
    return hashlib.sha256(output.read_bytes()).hexdigest()


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    inputs = parser.add_mutually_exclusive_group(required=True)
    inputs.add_argument('--firmware', type=Path)
    inputs.add_argument('--isp-library', type=Path, help='Already extracted original 1.2.36 ELF')
    parser.add_argument('--modules', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    if bool(args.firmware) != bool(args.modules):
        parser.error('--firmware requires --modules, and --isp-library takes no key')
    if args.firmware:
        sys.path.insert(0, str(HERE.parents[1]))
        from openx6.official import import_modules, reconstruct
        from openx6.fsimage import read_file
        with tempfile.TemporaryDirectory(dir=args.output.parent) as scratch:
            root = Path(scratch)
            stock = root/'stock'
            import_modules(args.firmware, args.modules, stock)
            image = root/'system.img'
            reconstruct(stock/'qcs8625_ota.zip', image)
            library = root/'isp.so'
            library.write_bytes(read_file(image, '/usr/lib/libinsispparam.so'))
            print(build(library, args.output))
    else:
        print(build(args.isp_library, args.output))
