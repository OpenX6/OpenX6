"""Change only both Camera Info display paths in an already verified 1.2.36 GUI."""
from .common import require

CALLBACK = 0x87D460
INITIAL = 0x8CD0A0
ORIGINAL_TEXT = b'1.2.36 S'
# The last instruction before the literal is an unconditional tail call.
# Its four bytes of following alignment padding can also hold display text.
CALLBACK_PREFIX = bytes.fromhex(
    '3f2303d5 fd7bbea9 fd030091 f30b00f9 f30300aa 40010010 '
    '829af197 e10300aa 02bc86d2 6002028b f30b40f9 fd7bc2a8 bf2303d5 fb960314')
INIT_PREFIX = bytes.fromhex('e01fd810 775bf097 e10300aa e00318aa f4570294 1f2003d5 1f2003d5')
ORIGINAL_TAIL = bytes.fromhex('1f2003d5') + ORIGINAL_TEXT.ljust(12, b'\0')


def camera_info_edits(gui, label):
    require(gui[CALLBACK:CALLBACK + 56] == CALLBACK_PREFIX and
            gui[CALLBACK + 56:CALLBACK + 72] == ORIGINAL_TAIL and
            gui[INITIAL:INITIAL + 28] == INIT_PREFIX, 'Unrecognized Camera Info display callbacks')
    require(isinstance(label, str) and label.isascii() and label.isprintable() and
            0 < len(label) < 16, 'Camera Info label must fit the 16-byte display field')
    if label.encode('ascii') == ORIGINAL_TEXT:
        return ()
    # Both ADR x0 instructions reference the same literal. Move it back four
    # bytes, retaining its NUL and the next function's original alignment.
    return ((CALLBACK + 20, bytes.fromhex('20010010')),
            (CALLBACK + 56, label.encode('ascii').ljust(16, b'\0')),
            (INITIAL, bytes.fromhex('c01fd810')))


def camera_info_gui(gui, label):
    edits = camera_info_edits(gui, label)
    if not edits:
        return gui
    result = bytearray(gui)
    for at, value in edits:
        result[at:at + len(value)] = value
    return bytes(result)
