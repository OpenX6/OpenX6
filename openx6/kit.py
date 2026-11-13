"""Verify the source-only component profile before constructing a unified image."""
import json
from pathlib import Path

from .common import asset, digest, require, sha
from .component_sources import SOURCES, inventory
from .release_recipe import verify_recipe

TRUST = Path(__file__).resolve().parents[1] / 'release'


def checked_inventory():
    pin = json.loads((TRUST / 'preview.json').read_bytes())
    profile = SOURCES / 'profile.json'
    body = profile.read_bytes()
    require(sha(body) == pin['profile_sha256'], 'Component profile mismatch.')
    verify_recipe(profile, TRUST / 'profile.sig', TRUST / 'component-preview.pub')
    definition = json.loads(body)
    recipes = SOURCES.parent/'recipes/single_capture'
    pins = definition['capture_recipes']
    require(set(pins) == {p.name for p in recipes.iterdir() if p.is_file()}, 'Capture recipe inventory differs')
    for name, checksum in pins.items():
        require(digest(asset(recipes, name)) == checksum, 'Capture recipe changed: '+name)
    expected = inventory(definition, pin['profile_sha256'])
    require(len(expected) == pin['files'], 'Invalid component inventory')
    return expected
