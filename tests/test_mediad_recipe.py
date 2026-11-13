"""Set OPENX6_TEST_STOCK_MEDIAD to a local official executable for the ARM build test."""
import json
import os
from pathlib import Path
import unittest
from unittest.mock import patch

from openx6 import mediad_recipe
from openx6.common import sha
from openx6.component_sources import SOURCES, render_stock
from openx6.patch import apply_recipe


class MediaRecipe(unittest.TestCase):
    def test_unrecognized_service_never_reaches_compiler(self):
        with patch.object(mediad_recipe, 'payload') as compiler:
            with self.assertRaisesRegex(ValueError, 'digest differs'):
                mediad_recipe.media_service(b'unknown', {})
            compiler.assert_not_called()

    @unittest.skipUnless(os.getenv('OPENX6_TEST_STOCK_MEDIAD') and os.getenv('OPENX6_TEST_STOCK_TREE'), 'requires official media service and stream profiles')
    def test_signed_delta_and_intermediate_rejection(self):
        stock = Path(os.environ['OPENX6_TEST_STOCK_MEDIAD']).read_bytes()
        profile = json.loads((SOURCES/'profile.json').read_bytes())
        action = profile['actions']['/usr/bin/ins_mediad']
        rule = json.loads((SOURCES/'stock-rules.json').read_bytes())[action['asset']]
        from openx6.single_capture import reconstruct
        from openx6.toolchain import compiler_tools
        tree = Path(os.environ['OPENX6_TEST_STOCK_TREE'])
        recipe = reconstruct(rule,stock,lambda name:(tree/name.lstrip('/')).read_bytes(),compiler_tools())
        self.assertEqual(sha(recipe), action['asset_sha256'])
        self.assertEqual(sha(apply_recipe(stock, json.loads(recipe))), action['sha256'])

        compile_payload = mediad_recipe.payload
        compiled = []
        def wrong_thumbnail(folder, base, sources, *args, **kwargs):
            compiled.extend(sources)
            result = compile_payload(folder, base, sources, *args, **kwargs)
            if 'thumbnail.S' in sources:
                return bytes([result[0] ^ 1]) + result[1:]
            return result

        with patch.object(mediad_recipe, 'payload', wrong_thumbnail):
            with self.assertRaisesRegex(ValueError, 'differs at preview thumbnail'):
                mediad_recipe.media_service(stock)
        self.assertEqual(compiled[-1], 'thumbnail.S')
