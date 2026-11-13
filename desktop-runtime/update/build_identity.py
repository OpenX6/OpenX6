"""Embed the same release coordinates in the host updater and desktop package."""
import json
import os
from pathlib import Path
import re

PREFIX = 'openx6.local/desktop/update.'


def identity():
    repo = os.environ.get('OPENX6_UPDATE_REPOSITORY', '')
    version = os.environ.get('OPENX6_DESKTOP_VERSION', '')
    if not repo and not version:
        default = Path(__file__).resolve().parents[2]/'release/desktop-channel.json'
        data = json.loads(default.read_bytes())
        repo, version = data['repository'], data['version']
    if (not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9][A-Za-z0-9_.-]*', repo)
            or not re.fullmatch(r'(0|[1-9][0-9]{0,8})(\.(0|[1-9][0-9]{0,8})){2}', version)):
        raise ValueError('Set a GitHub owner/repository and a numeric X.Y.Z desktop version together.')
    return {'repository': repo, 'version': version}


def stamp(folder):
    data = identity()
    (Path(folder)/'desktop-release.json').write_text(json.dumps(data)+'\n')
    return f"-X {PREFIX}Repository={data['repository']} -X {PREFIX}Version={data['version']}"
