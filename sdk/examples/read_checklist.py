"""Read the field checklist as a separate consumer of a module-owned setting."""
import argparse
import json
from pathlib import Path

from sdk.choice import read_choice


def selected_note(directory):
    manifest = json.loads(Path(__file__).with_name('field_checklist.json').read_bytes())
    selected = read_choice(directory, manifest)
    return next(option['note'] for option in manifest['setting']['options'] if option['id'] == selected)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('preferences', type=Path, help='feature preference directory or local test fixture')
    args = parser.parse_args()
    if not args.preferences.is_dir():
        parser.error('choose an existing preference directory')
    try:
        print(selected_note(args.preferences))
    except (OSError, ValueError) as exc:
        parser.exit(1, f'Could not read checklist: {exc}\n')
