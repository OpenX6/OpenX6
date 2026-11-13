"""Replay the 1.2.36 menu constructor from an installed GUI ELF (requires Unicorn)."""
import argparse
import json
import struct
from pathlib import Path

from unicorn import UC_ARCH_ARM64, UC_HOOK_CODE, UC_HOOK_INTR, UC_MODE_ARM, Uc
from unicorn.arm64_const import UC_ARM64_REG_LR, UC_ARM64_REG_PC, UC_ARM64_REG_SP, UC_ARM64_REG_X0, UC_ARM64_REG_X8


def menu_rows(image, saved, *, fail_page=False):
    cpu = Uc(UC_ARCH_ARM64, UC_MODE_ARM)
    header = struct.Struct("<II6Q")
    for i in range(struct.unpack_from("<H", image, 56)[0]):
        kind, _, offset, addr, _, size, memsize, _ = header.unpack_from(image, 64 + header.size * i)
        if kind == 1 and memsize:
            start = addr & ~4095
            cpu.mem_map(start, ((addr + memsize + 4095) & ~4095) - start)
            cpu.mem_write(addr, image[offset:offset + size])
    heap = 0x30000000
    cpu.mem_map(heap, 0x100000)
    parent, child, stop = heap, heap + 0x2000, heap + 0x4000
    cursor = heap + 0x6000
    rows, titles, checked, labels = [], {}, {}, {}
    label = "Disable Flat Mode Mirror"
    pref = "/data/x6-owner-features/flat-mirror-v1/enabled"
    symbols = json.loads((Path(__file__).resolve().parents[1] / "recipes/single_menu/menu-symbols.json").read_text())

    def put(addr, value):
        cpu.mem_write(addr, struct.pack("<Q", value))

    def reg(n):
        return cpu.reg_read(UC_ARM64_REG_X0 + n)

    def string(addr):
        return bytes(cpu.mem_read(addr, 256)).split(b"\0", 1)[0].decode()

    def widget(_, pc, size, name):
        nonlocal cursor
        arg, value = reg(0), reg(1)
        result = 0
        if name == "create_subpage" and not fail_page:
            put(arg + 0xa8, child)
            put(child, value)
        elif name == "new_object":
            cursor += 0x1000
            result = cursor
            put(result, arg)
        elif name in ("new_string", "utf8_string"):
            labels[arg] = string(arg)
            result = arg
        elif name == "page_title":
            assert labels[value] == "Mod Settings"
        elif name == "group_add":
            assert arg == child
            rows.append(value)
        elif name in ("switch_title", "arrow_title"):
            titles[arg] = labels[value]
        elif name == "switch_checked":
            checked[arg] = value
        elif name == "new_slot":
            result = arg
        cpu.reg_write(UC_ARM64_REG_X0, result)
        cpu.reg_write(UC_ARM64_REG_PC, cpu.reg_read(UC_ARM64_REG_LR))

    for name in ("create_subpage", "new_object", "new_string", "page_title", "group_add",
                 "switch_title", "arrow_title", "switch_checked", "new_slot", "utf8_string"):
        pc = 0x4e4010 if name == "utf8_string" else int(symbols[name], 16)
        cpu.hook_add(UC_HOOK_CODE, widget, name, pc, pc)

    def syscall(_, number, data):
        op = cpu.reg_read(UC_ARM64_REG_X8)
        if op == 48:  # faccessat
            result = 0 if saved and string(reg(1)) == pref else -2
        elif op == 56:  # other rows can read runtime state
            assert reg(2) & (3 | 0x40 | 0x200) == 0, "opening the menu attempted a write"
            result = -2
        else:
            raise AssertionError(f"unexpected menu syscall {op}")
        cpu.reg_write(UC_ARM64_REG_X0, result & ((1 << 64) - 1))

    cpu.hook_add(UC_HOOK_INTR, syscall)
    for _ in range(2):  # second open sees the existing child
        cpu.reg_write(UC_ARM64_REG_X0, parent)
        cpu.reg_write(UC_ARM64_REG_SP, heap + 0xfe000)
        cpu.reg_write(UC_ARM64_REG_LR, stop)
        cpu.emu_start(0x7009078, stop, count=30000)
        assert cpu.reg_read(UC_ARM64_REG_PC) == stop
    names = [titles[row] for row in rows]
    if fail_page:
        assert not names
    else:
        assert names.count(label) == 1 and names[-1] == label, f"mirror must be last: {names}"
        assert names[0] == "120 MP Sync"
        assert checked[rows[-1]] == int(saved)
    return names


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("gui", type=Path)
    image = parser.parse_args().gui.read_bytes()
    assert menu_rows(image, False) == menu_rows(image, True)
    menu_rows(image, False, fail_page=True)
    print("Menu order and both saved switch states passed.")
