#!/usr/bin/env python3
"""Convert released OpenX6 1.1 .x6b captures to per-eye CinemaDNG sequences."""
import argparse
import csv
import json
import shutil
import statistics
import sys
from fractions import Fraction
from pathlib import Path

from decoder import scan, decode_eye
from dng import write


def load_json(path):
    return json.loads(path.read_text(encoding='utf-8')) if path.exists() else {}


def rate(records, clip):
    magic = records[0].magic
    expected = 50 if magic.startswith(b'X6M50') else 60 if magic.startswith(b'X6Q') else 30
    supplied = clip.get('fps_setting', expected)
    if supplied != expected:
        raise ValueError('clip.json frame rate disagrees with the recorded codec')
    periods = [(b.stamps[0]-a.stamps[0]) / ((b.hardware-a.hardware) if a.single else (b.request-a.request))
               for a, b in zip(records, records[1:])]
    # Use actual exposure cadence, selecting between integer and 1000/1001 rates.
    candidates = [Fraction(expected), Fraction(expected*1000, 1001)]
    if periods:
        period = statistics.median(periods)
        result = min(candidates, key=lambda f: abs(1e9/float(f)-period))
        if abs(1e9/float(result)-period) > period * .01:
            raise ValueError('Exposure cadence is not a supported 1.1 frame rate')
        return result
    return candidates[1] if expected in (30, 60) else candidates[0]


def exposures(folder):
    path = folder/'exposure.csv'
    result = {}
    if path.exists():
        with path.open(newline='', encoding='utf-8') as f:
            for row in csv.DictReader(f):
                request = int(row['request'])
                if request in result:
                    raise ValueError('Duplicate exposure.csv request')
                result[request] = row
    return result


def convert(source, output, start=0, count=None, eye='both', gaps='hold',
            progress=print, cancelled=lambda: False):
    source, output = Path(source).resolve(), Path(output).resolve()
    if source.is_dir():
        source = source/'capture.x6b'
    if not source.is_file() or source.suffix.lower() != '.x6b':
        raise ValueError('Select a .x6b file or a RAWVIDEO folder containing capture.x6b')
    if output == source.parent or source.parent in output.parents:
        raise ValueError('Choose an output folder outside the source take')
    if output.exists():
        raise FileExistsError('Output must be a new folder; existing files are never overwritten')
    if start < 0 or (count is not None and count < 1):
        raise ValueError('Start must be nonnegative; count must be positive')
    before = source.stat()
    progress('Reading frame headers…')
    all_records = scan(source, cancelled)
    clip = load_json(source.parent/'clip.json')
    fps = rate(all_records, clip)
    records = all_records[start:None if count is None else start+count]
    if not records:
        raise ValueError('No frames in requested range')
    if clip.get('size', [records[0].width, records[0].height]) != [records[0].width, records[0].height]:
        raise ValueError('clip.json dimensions disagree with capture')
    single = records[0].single
    channel = clip.get('eye', 'SINGLE') if single else None
    if single and channel not in ('CAM0', 'CAM1', 'SINGLE'):
        raise ValueError('Unknown single-eye channel')
    if single and eye != 'both' and channel != eye:
        raise ValueError('Requested eye is absent or not identified in clip.json')
    eyes = [(0, channel)] if single else [(i, f'CAM{i}') for i in range(2) if eye in ('both', f'CAM{i}')]
    exposure = exposures(source.parent)
    def frame_id(r):
        return r.hardware if r.single else r.request
    missing = sum(frame_id(b)-frame_id(a)-1 for a, b in zip(records, records[1:]))
    if missing and gaps == 'error':
        raise ValueError(f'{missing} missing frames; choose hold or keep explicitly')
    if missing > 10000:
        raise ValueError('More than 10000 missing frames; inspect the recording before conversion')
    total = len(records) + (missing if gaps == 'hold' else 0)
    needed = total * records[0].width * records[0].height * 2 * len(eyes)
    ancestor = output.parent
    while not ancestor.exists():
        ancestor = ancestor.parent
    if shutil.disk_usage(ancestor).free < needed + 64*1024*1024:
        raise ValueError(f'Not enough free space: DNG pixels alone need {needed/1e9:.2f} GB')
    progress(f'{len(records)} source frames, {missing} gaps; {fps} fps; about {needed/1e9:.2f} GB output plus audio')
    output.mkdir(parents=True)
    for _, name in eyes:
        (output/name).mkdir()
    report = dict(status='incomplete', source=str(source), codec=records[0].magic.rstrip(b'\0').decode(),
                  fps=str(fps), source_frames=len(records), missing_frames=missing, gap_policy=gaps,
                  single_eye=single, sensor_bits=10 if single else 14, dng_storage_bits=16,
                  color='development reference, not measured per-take white balance',
                  timestamps='original device exposure clock in nanoseconds; see copied clock metadata',
                  converted_source_frames=0, written_dngs=0,
                  crc='checked wherever flagged in single-eye headers; dual-eye format has no pixel CRC')
    report_path = output/'conversion.json'
    def save_report():
        report_path.write_text(json.dumps(report, indent=2)+'\n', encoding='utf-8')
    save_report()
    try:
        metadata = output/'metadata'; metadata.mkdir()
        # Keep all sidecars (including future schemas), but never copy the RAW payload.
        for item in sorted(source.parent.iterdir()):
            if item.is_file() and not item.is_symlink() and item.suffix.lower() in ('.json', '.csv', '.aac', '.wav', '.txt'):
                if cancelled():
                    raise InterruptedError('Cancelled')
                shutil.copy2(item, metadata/item.name)
        if not clip:
            report['warning'] = 'clip.json missing: reference color/CFA defaults used; single-eye channel may be unknown'
        with source.open('rb') as f, (output/'frames.csv').open('w', newline='', encoding='utf-8') as mapping:
            writer = csv.writer(mapping)
            writer.writerow(['sequence', 'eye', 'file', 'held_previous', 'source_request', 'hardware_frame',
                             'exposure_start_ns', 'exposure_duration_ns', 'source_offset'])
            previous = {}
            sequence = 0
            last_id = None
            for i, record in enumerate(records):
                if cancelled():
                    raise InterruptedError('Cancelled')
                gap = frame_id(record)-last_id-1 if last_id is not None else 0
                for _ in range(gap):
                    if cancelled():
                        raise InterruptedError('Cancelled')
                    if gaps == 'hold':
                        for _, name in eyes:
                            path = output/name/f'frame_{sequence:08d}.dng'
                            old_path, row = previous[name]
                            shutil.copyfile(old_path, path)
                            writer.writerow([sequence, name, str(path.relative_to(output)), True, *row])
                            report['written_dngs'] += 1
                    sequence += 1
                for index, name in eyes:
                    pixels = decode_eye(f, record, index)
                    duration = record.duration if single else int(exposure.get(record.request, {}).get(f'cam{index}_exposure_ns') or 0)
                    path = output/name/f'frame_{sequence:08d}.dng'
                    row = [record.request, record.hardware if single else '', record.stamps[index], duration or '', record.offset]
                    write(path, pixels, clip, fps,
                          f'{name} request={record.request} exposure_start_ns={record.stamps[index]}', duration)
                    previous[name] = path, row
                    writer.writerow([sequence, name, str(path.relative_to(output)), False, *row])
                    report['written_dngs'] += 1
                sequence += 1
                last_id = frame_id(record)
                report['converted_source_frames'] = i+1
                mapping.flush()
                progress(f'{i+1}/{len(records)} source frames converted')
            after = source.stat()
            if (before.st_size, before.st_mtime_ns, before.st_ino) != (after.st_size, after.st_mtime_ns, after.st_ino):
                raise ValueError('Source changed during conversion; output is incomplete')
        report['status'] = 'complete'
        save_report()
    except BaseException as exc:
        report['status'] = 'cancelled' if isinstance(exc, (InterruptedError, KeyboardInterrupt)) else 'failed'
        report['error'] = str(exc)
        save_report()
        raise
    progress(f'Done: {output}')
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', help='.x6b file or RAWVIDEO folder')
    parser.add_argument('output', help='new destination folder outside the source take')
    parser.add_argument('--start', type=int, default=0, help='zero-based captured frame index')
    parser.add_argument('--count', type=int, help='number of captured frames to convert')
    parser.add_argument('--eye', choices=['both', 'CAM0', 'CAM1'], default='both')
    parser.add_argument('--gaps', choices=['hold', 'keep', 'error'], default='hold',
                        help='hold: repeat previous frame and log it; keep: leave filename holes; error: refuse')
    args = parser.parse_args()
    try:
        convert(**vars(args))
    except (OSError, ValueError, InterruptedError, KeyboardInterrupt) as exc:
        print(f'Conversion stopped: {exc}', file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
