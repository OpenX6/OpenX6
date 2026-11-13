import struct
import unittest

from openx6.common import Rejected
from openx6.fisheye import MENU_DONE, MENU_TAIL, check_menu_order
from openx6.raw_nr_recipe import branch


class MenuOrderTests(unittest.TestCase):
    def test_only_completed_last_row_can_append_mirror(self):
        start = 0x7009000
        target = MENU_DONE + 0x8000
        rx = (1, 5, 0, start)
        words = {
            0x70090e0: branch(0x70090e0, 0x761eb0) | 0x80000000,
            0x70094d0: branch(0x70094d0, 0x700d3a0),
            MENU_TAIL - 8: 0x91130289,
            MENU_TAIL - 4: 0xa9000520,
            MENU_TAIL: branch(MENU_TAIL, target),
            MENU_DONE: 0xa94153f3,
        }
        image = bytearray(MENU_TAIL - start + 4)
        for pc, value in words.items():
            struct.pack_into("<I", image, pc - start, value)
        check_menu_order(image, rx, target)
        for pc, value in ((0x70090e0, branch(0x70090e0, target) | 0x80000000),
                          (MENU_TAIL, branch(MENU_TAIL, MENU_DONE)),
                          (MENU_TAIL - 4, 0xd503201f),
                          (MENU_DONE, branch(MENU_DONE, target))):
            with self.subTest(pc=hex(pc)):
                modified = bytearray(image)
                struct.pack_into("<I", modified, pc - start, value)
                with self.assertRaisesRegex(Rejected, "Mod Settings"):
                    check_menu_order(modified, rx, target)
