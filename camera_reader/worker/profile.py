"""Stock updater identities accepted by the isolated module exporter."""
import hashlib
from pathlib import Path


UPDATERS = {
    '0.8.825': 'ba6001f68643c5c2bf4d9c86c72b3925e71583c9a123b112186b05b9e4a7688e',
    '0.8.827': 'd2ef51479725950986b9ad45a82ed2a7c60a8accafeef58a46f795b8f770531b',
}


def check_updater(version, checksum):
    if version not in UPDATERS or UPDATERS[version] != checksum:
        raise ValueError('Unsupported X6 firmware and updater combination / 固件版本与升级器摘要不匹配')


def check_installed(version, version_file, updater_file):
    prefix = 'qcs8625_Version:V'
    versions = [line[len(prefix):] for line in Path(version_file).read_text().splitlines()
                if line.startswith(prefix)]
    if versions != [version]:
        raise ValueError('Installed firmware differs from the authorized camera version')
    check_updater(version, hashlib.sha256(Path(updater_file).read_bytes()).hexdigest())
