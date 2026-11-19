import hashlib
import struct
import unittest
from unittest.mock import patch

from openx6 import raw_menu
from openx6.common import Rejected


class RawTimerLayoutTests(unittest.TestCase):
    def setUp(self):
        self.site = 0x40
        self.reference = 0x2050
        self.current = 0x1030
        self.previous = 0x6094
        self.callback = 0x7150
        self.header_at = 64 + 3 * raw_menu.HEADER.size
        self.gui = bytearray(0x2600)
        self.template = {
            'end': len(self.gui),
            'edits': [
                [self.reference,
                 raw_menu.timer_stub(self.reference, self.previous).hex(),
                 raw_menu.timer_stub(self.reference, self.callback).hex()],
                [self.header_at, self.header(0x1200).hex(), self.header(0x1300).hex()],
            ],
        }
        self.gui[self.header_at:self.header_at+56] = self.header(0x1100)
        struct.pack_into('<I',self.gui,self.site,0x94000000 | (self.current-self.site)//4)
        self.gui[self.current:self.current+12] = (
            raw_menu.timer_stub(self.current,self.previous) + bytes.fromhex('00021fd6'))

    @staticmethod
    def header(length):
        return raw_menu.HEADER.pack(1,5,0x1000,0x1000,0x1000,length,length,4096)

    def bind(self):
        with patch.object(raw_menu,'SITE',self.site):
            return raw_menu.bind_recipe(self.template,self.gui)

    def test_different_layout_keeps_timer_chain_and_correct_hash(self):
        result = self.bind()
        self.assertEqual(result['pin'],hashlib.sha256(self.gui).hexdigest())
        site, before, after = result['edits'][0]
        self.assertEqual(site,self.current)
        self.assertEqual(bytes.fromhex(before),self.gui[site:site+8])
        self.assertEqual(bytes.fromhex(after),raw_menu.timer_stub(self.current,self.callback))
        self.assertEqual(result['edits'][1][1],self.header(0x1100).hex())
        self.assertEqual(self.template['edits'][0][0],self.reference)

    def test_rejects_wrong_previous_callback(self):
        self.gui[self.current:self.current+8] = raw_menu.timer_stub(self.current,self.previous+4)
        with self.assertRaisesRegex(Rejected,'timer chain'):
            self.bind()

    def test_rejects_overlap_with_existing_menu(self):
        self.gui[self.header_at:self.header_at+56] = self.header(0x1280)
        with self.assertRaisesRegex(Rejected,'Insufficient space'):
            self.bind()
