#!/usr/bin/env python3
"""Frozen GUI<->daemon + journal contract for the PureShot post-processing page (DESIGN-v3 §4/§5.3).

Single source of truth for the byte layouts, op/state/result codes, tags and on-device paths. Both the
daemon (x6-pureshot-processor) and the offline tests import this. The GUI (menu.S) hard-codes the same
offsets/sizes; CONTRACT.md documents them for the assembly side. Pure stdlib, no camera dependency,
Python 3.10 compatible (the camera's interpreter). PYTHONDONTWRITEBYTECODE=1 / -B when running.

NOTE on the groups record: DESIGN-v3 §4.3's record field list summed to 66 bytes (a doc slip). The
frozen record is 64 bytes: u32 id, u16 total, u16 done, u16 failed, u8 flags, u8 storage, u32 pad,
char title[48]. CONTRACT.md carries the authoritative map; DESIGN-v3 §4.3 is annotated to match.
"""
import struct

MAGIC = b'PSG1'

# ---- request: GUI -> daemon, 8 bytes, written via staged .next + renameat2(RENAME_NOREPLACE) ----
REQ_FMT = '<BBBBI'            # op, arg, gen_lo, 'P', id
REQ_SIZE = 8
REQ_MARK = ord('P')
OP_SELECT     = 0x01          # arg=slot, id=group id
OP_DESELECT   = 0x02          # arg=slot, id=group id
OP_SELECT_ALL = 0x03
OP_SELECT_NONE= 0x04
OP_START      = 0x10
OP_STOP       = 0x11
OP_RESCAN_ACK = 0x20          # rescan storage; in state 9 also acknowledges the error/STUCK
OP_PAGE       = 0x21          # advance the group window by 8 (wraps)
OPS = {OP_SELECT, OP_DESELECT, OP_SELECT_ALL, OP_SELECT_NONE, OP_START, OP_STOP, OP_RESCAN_ACK, OP_PAGE}

def pack_request(op, arg=0, gen_lo=0, gid=0):
    return struct.pack(REQ_FMT, op & 0xff, arg & 0xff, gen_lo & 0xff, REQ_MARK, gid & 0xffffffff)

def parse_request(b):
    """Return dict or None if the 8-byte record is structurally invalid."""
    if len(b) != REQ_SIZE:
        return None
    op, arg, gen_lo, mark, gid = struct.unpack(REQ_FMT, b)
    if mark != REQ_MARK or op not in OPS:
        return None
    return {'op': op, 'arg': arg, 'gen_lo': gen_lo, 'id': gid}

# ---- status: daemon -> GUI, 16-byte header + NUL-terminated text, total 18..255 (stats-overlay shape) ----
STATUS_HDR_FMT = '<QIBBBB'    # mono_s(u64), severity(u32), state(u8), percent(u8), req_seq(u8), last_result(u8)
STATUS_HDR = 16
STATUS_MIN, STATUS_MAX = 18, 255
STATUS_TEXT_MAX = STATUS_MAX - STATUS_HDR - 1   # 238 bytes of text before the NUL
SEV_NORMAL, SEV_PAUSED, SEV_ERROR, SEV_GREY = 0, 1, 2, 3
ST_NOT_READY, ST_NO_GROUPS, ST_SCANNING, ST_READY, ST_RUNNING, ST_PAUSED, ST_STOPPING, ST_STOPPED, ST_DONE, ST_ERROR = range(10)
RES_OK, RES_STALE, RES_BUSY, RES_INVALID, RES_NOTHING, RES_BLOCKED = range(6)

def pack_status(mono_s, severity, state, percent, req_seq, last_result, text):
    t = text.encode('utf-8', 'replace') if isinstance(text, str) else bytes(text)
    t = t[:STATUS_TEXT_MAX]
    body = struct.pack(STATUS_HDR_FMT, int(mono_s) & 0xffffffffffffffff, severity & 0xffffffff,
                       state & 0xff, max(0, min(100, percent)) & 0xff, req_seq & 0xff, last_result & 0xff) + t + b'\0'
    if len(body) < STATUS_MIN:          # pad the text so even an empty message yields a legal record
        body = body[:-1] + b' ' * (STATUS_MIN - len(body)) + b'\0'
    return body

# ---- groups: daemon -> GUI, exactly 544 bytes = 32-byte header + 8 x 64-byte records ----
GROUPS_SIZE = 544
GRP_HDR_FMT = '<4sIBBBBII12x'   # magic, gen, count, flags, req_seq, window_lo, total_groups, window_off, 12 pad
GRP_HDR = 32
GRP_REC_FMT = '<IHHHBB4x48s'    # id, total, done, failed, flags, storage, 4 pad, title[48]
GRP_REC = 64
GRP_MAX = 8                     # records that fit; count in [0,8]
assert struct.calcsize(GRP_HDR_FMT) == GRP_HDR and struct.calcsize(GRP_REC_FMT) == GRP_REC
assert GRP_HDR + GRP_MAX * GRP_REC == GROUPS_SIZE
# header flags
GF_MORE, GF_SCANNING, GF_EXPORT, GF_BLOCKED = 1, 2, 4, 8
# record flags
RF_SELECTED, RF_CURRENT, RF_COMPLETE, RF_FAILURES, RF_MISSING = 1, 2, 4, 8, 16
# storage enum
STO_INTERNAL, STO_SD, STO_SSD = 0, 1, 2

def _pack_record(r):
    title = r.get('title', '')
    tb = title.encode('utf-8', 'replace')[:47] if isinstance(title, str) else bytes(title)[:47]
    return struct.pack(GRP_REC_FMT, r['id'] & 0xffffffff, r.get('total', 0) & 0xffff,
                       r.get('done', 0) & 0xffff, r.get('failed', 0) & 0xffff,
                       r.get('flags', 0) & 0xff, r.get('storage', 0) & 0xff, tb)

def pack_groups(gen, flags, req_seq, window_lo, total_groups, window_off, records):
    """records: list of dicts (<=8) with id,total,done,failed,flags,storage,title. Pads to 544."""
    out = struct.pack(GRP_HDR_FMT, MAGIC, gen & 0xffffffff, min(len(records), GRP_MAX) & 0xff, flags & 0xff,
                      req_seq & 0xff, window_lo & 0xff, total_groups & 0xffffffff, window_off & 0xffffffff)
    for r in records[:GRP_MAX]:
        out += _pack_record(r)
    out += b'\0' * (GROUPS_SIZE - len(out))
    return out

# ---- groups-all: daemon -> GUI, the whole list for the stacked "older groups" pages (owner 2026-11-04) ----
# Same 32-byte header (magic 'PSGA'; count = records in THIS file, <= GRP_ALL_MAX; window fields 0) followed by
# count x 64-byte records, newest group first, no padding: size = 32 + 64 x count exactly. The GUI page at depth d
# reads records 8d..8d+7. The 544-byte 'groups' file (first 8) is still written for a GUI that predates this.
MAGIC_ALL = b'PSGA'
GRP_ALL_MAX = 128               # 16 pages of 8 on the camera (the GUI's page stack depth)

def pack_groups_all(gen, flags, req_seq, total_groups, records):
    recs = records[:GRP_ALL_MAX]
    out = struct.pack(GRP_HDR_FMT, MAGIC_ALL, gen & 0xffffffff, len(recs) & 0xff, flags & 0xff,
                      req_seq & 0xff, 0, total_groups & 0xffffffff, 0)
    return out + b''.join(_pack_record(r) for r in recs)

# ---- page heartbeat: GUI -> daemon, 16 bytes {u64 CLOCK_MONOTONIC seconds, 'PPGO', u32 depth}, renamed over
# RUN_PAGE about once a second while one of the pages is in front; removed when none is ----
PAGE_FMT = '<Q4sI'
PAGE_SIZE = 16
PAGE_MAGIC = b'PPGO'
assert struct.calcsize(PAGE_FMT) == PAGE_SIZE

def pack_page(mono_s, depth=0):
    return struct.pack(PAGE_FMT, int(mono_s) & 0xffffffffffffffff, PAGE_MAGIC, depth & 0xffffffff)

# ---- journal entry states (J on /data, and the on-volume .x6j mirror) ----
J_CONVERTING, J_VERIFIED, J_WRITING, J_WRITTEN, J_STUCK = 'CONVERTING', 'VERIFIED', 'WRITING', 'WRITTEN', 'STUCK'

# ---- passthrough / converted DNG markers ----
WHITE_PASSTHROUGH = 1023
WHITE_CONVERTED = 16383
DNG_W, DNG_H, DNG_BPS, DNG_COMPRESSION, DNG_STRIPS = 15520, 7760, 16, 8, 31
MIN_DNG_BYTES = 100_000_000

# ---- on-device paths ----
RUN = '/run/x6-pureshot'
RUN_REQUEST = RUN + '/request'
RUN_REQUEST_NEXT = RUN + '/request.next'
RUN_STATUS = RUN + '/status'
RUN_GROUPS = RUN + '/groups'
RUN_GROUPS_ALL = RUN + '/groups-all'
RUN_PAGE = RUN + '/page'
RUN_LOCK = RUN + '/lock'
RUN_DIAG = RUN + '/diagnostic.json'
DATA = '/data/x6-owner-features/pureshot-v1'
DATA_JOB = DATA + '/job.json'
DATA_JOURNAL = DATA + '/journal'
DATA_STUCK = DATA + '/journal/stuck'
DATA_SPEED = DATA + '/storage-speed.json'
LIB_DIR = '/usr/lib/x6-pureshot'        # the firmware installs the daemon modules, ps2native and the batch tool here
PS2NATIVE = LIB_DIR + '/ps2native'
BATCH_DIR = '/data/x6-owner-features/pureshot-batch-v1'   # the hot-patch era location (unit 5 before v24); unused

# per-volume work dir (relative to a volume root), DESIGN-v3 §5.3
WORKDIR = '.x6-pureshot'            # <volroot>/.x6-pureshot
QUARANTINE = WORKDIR + '/quarantine'
def tmp_name(stem):  return stem + '.x6n'          # ps2native OUT; it writes <OUT>.tmp then renames
def tmp_tmp(stem):   return stem + '.x6n.tmp'
def xj_name(stem):   return stem + '.x6j'          # on-volume journal mirror

# storage roots (topmost mount must be rw exfat; SSD sits over a ro tmpfs guard)
STORAGE_ROOTS = (
    ('/ins/media_rw/emulated', STO_INTERNAL),
    ('/mnt/sdcard', STO_SD),
    ('/mnt/quickreader', STO_SSD),
)

# arm64 open flags the GUI uses (documented for CONTRACT.md / cross-check). These are the VALUES the proven
# on-device vr_*/nr_* menu.S code uses. Correction (2026-11-04): on aarch64 0x20000 is O_LARGEFILE and O_NOFOLLOW
# is 0x8000 (arch/arm64 uapi fcntl.h), so these two do NOT carry O_NOFOLLOW; the readers rely on their fstat checks
# (regular, one link, uid 0) inside the root-only /run/x6-pureshot. The page heartbeat writer uses 0xa8241, which
# does. Never used by the daemon (it opens with symbolic os.O_*).
O_RD_NOFOLLOW = 0xa0000        # O_RDONLY|O_LARGEFILE|O_CLOEXEC (name kept for the existing references)
O_STAGE_WRITE = 0xa00c1        # O_WRONLY|O_CREAT|O_EXCL|O_LARGEFILE|O_CLOEXEC
O_BEAT_WRITE = 0xa8241         # O_WRONLY|O_CREAT|O_TRUNC|O_NOFOLLOW|O_LARGEFILE|O_CLOEXEC
RENAME_NOREPLACE = 1           # renameat2 flag used to stage the request atomically
