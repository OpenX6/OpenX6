"""Create and check OpenX6 modules from a source checkout."""
import argparse
import json
from pathlib import Path

from openx6.build import preflight_modules
from sdk import choice
from sdk.module_plan import ID


def create_module(directory):
    name = directory.name
    if not ID.fullmatch(name):
        raise ValueError('Use a directory name of 1–48 lowercase letters, digits or underscores, starting with a letter.')
    example = Path(__file__).parent/'examples'/'recording_budget'/'module.json'
    (doc,), sources = preflight_modules([example], full=True)
    handler = sources[doc['id']]
    doc['id'] = name
    doc['title'] = name.replace('_', ' ').strip().capitalize()
    choice.validate(doc)
    files = {
        doc['program']['source']: handler,
        'module.json': (json.dumps(doc, indent=2) + '\n').encode(),
        'LICENSE': Path(__file__).with_name('LICENSE').read_bytes(),
    }
    directory.mkdir()  # Refuse existing directories, even an empty one.
    created = []
    try:
        for filename, content in files.items():
            path = directory/filename
            with path.open('xb') as output:
                created.append(path)
                output.write(content)
    except OSError:
        for path in created:
            path.unlink(missing_ok=True)
        directory.rmdir()
        raise
    return directory/'module.json'


def main():
    parser = argparse.ArgumentParser(prog='python -m sdk', description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    init = commands.add_parser('init', help='Create an editable menu and Python handler')
    init.add_argument('directory', type=Path, help='New directory; the final component becomes the module ID')
    check = commands.add_parser('check', help='Check a selection without running its code or opening firmware')
    check.add_argument('--profile', choices=('full', 'stock'), default='full')
    check.add_argument('manifests', nargs='+', type=Path)
    args = parser.parse_args()
    try:
        if args.command == 'init':
            manifest = create_module(args.directory)
            print(f'Created {manifest}. Edit the menu in module.json and the calculation in budget.py.')
            print('Run python -m sdk check with the path to the new manifest.')
        else:
            mods, sources = preflight_modules(args.manifests, full=args.profile == 'full')
            print(json.dumps({'profile': args.profile, 'modules': [m['id'] for m in mods],
                              'checked_handlers': sorted(sources)}, indent=2))
    except (OSError, ValueError, RecursionError) as exc:
        parser.exit(1, f'OpenX6 module {args.command}: {exc}\n')


if __name__ == '__main__':
    main()
