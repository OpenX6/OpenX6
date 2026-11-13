"""Declarative native radio setting with a per-choice note and stable preference IDs."""
try:
    from .module_plan import ID, check
except ImportError:
    from module_plan import ID, check

SCHEMA = 'openx6.choice.v1'


def identifier(value):
    check(isinstance(value, str) and ID.fullmatch(value), 'invalid choice identifier')


def caption(value):
    check(isinstance(value, str) and 0 < len(value) <= 48 and value.isascii()
          and value.isprintable() and value == value.strip(), 'expected printable ASCII caption')


def validate(doc):
    fields = set(doc) - {'program'} if isinstance(doc, dict) else set()
    check(isinstance(doc, dict) and fields == {'schema', 'id', 'title', 'setting'}
          and doc['schema'] == SCHEMA, 'invalid choice module')
    if 'program' in doc:
        import re
        program = doc['program']
        check(isinstance(program, dict) and set(program) == {'api','source','sha256'}
              and program['api'] == 'openx6.python.choice.v1', 'unsupported executable module API')
        check(isinstance(program['source'], str) and re.fullmatch(r'[a-z][a-z0-9_]{0,47}\.py', program['source']),
              'expected a sibling Python source file')
        check(isinstance(program['sha256'], str) and re.fullmatch(r'[a-f0-9]{64}', program['sha256']),
              'expected source SHA-256')
    identifier(doc['id'])
    caption(doc['title'])
    row = doc['setting']
    check(isinstance(row, dict) and set(row) == {'id','label','default','options'}, 'invalid choice setting')
    identifier(row['id'])
    caption(row['label'])
    opts = row['options']
    check(isinstance(opts, list) and 2 <= len(opts) <= 8, 'expected 2–8 options')
    for opt in opts:
        check(isinstance(opt, dict) and set(opt) == {'id','label','note'}, 'invalid choice option')
        identifier(opt['id'])
        caption(opt['label'])
        caption(opt['note'])
    for field in ('id', 'label'):
        check(len({o[field] for o in opts}) == len(opts), 'duplicate option '+field)
    check(row['default'] in [o['id'] for o in opts], 'unknown default option')


def read_choice(directory, doc):
    """Module-side API. Supply the feature directory or a local fixture directory."""
    validate(doc)
    row = doc['setting']
    path = directory / f"openx6-choice.{doc['id']}.{row['id']}.pref"
    import os
    import stat
    try:
        fd = os.open(path, os.O_NOFOLLOW | os.O_RDONLY | os.O_NONBLOCK)
    except FileNotFoundError:
        return row['default']
    with os.fdopen(fd, 'rb') as f:
        st = os.fstat(f.fileno())
        check(stat.S_ISREG(st.st_mode) and stat.S_IMODE(st.st_mode) == 0o600
              and st.st_uid == os.geteuid() and st.st_nlink == 1, 'unsafe preference file')
        value = f.read(49)
    for item in row['options']:
        if item['id'].encode() == value:
            return item['id']
    raise ValueError('unrecognized saved choice')
