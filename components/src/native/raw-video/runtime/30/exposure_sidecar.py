"""Export sensor-applied durations for exactly the requests present in the sealed take."""
import os

HEADER = 'request,cam0_exposure_ns,cam1_exposure_ns\n'


def finalize(folder, disk_log, first, last, expected):
    saved = bytearray(last - first + 1)
    with disk_log.open() as log:
        for line in log:
            if line.startswith('disk,'):
                index = int(line.split(',', 2)[1]) - first
                if not 0 <= index < len(saved) or saved[index]:
                    raise ValueError('Duplicate or out-of-window disk request')
                saved[index] = 1
    if saved.count(1) != expected:
        raise ValueError('Exposure index disagrees with flushed frame count')
    source = folder/'exposure.part'
    target = folder/'exposure.csv.tmp'
    missing = 0
    with source.open() as inp, target.open('x') as out:
        out.write(HEADER)
        for line in inp:
            req, left, right = line.rstrip('\n').split(',')
            index = int(req) - first
            if not 0 <= index < len(saved):
                raise ValueError('Exposure outside capture window')
            if saved[index]:
                out.write(line)
                missing += int(not (left and right))
                saved[index] = 0
        if any(saved):
            raise ValueError('Missing exposure record for written frame')
        out.flush()
        os.fsync(out.fileno())
    target.replace(folder/'exposure.csv')
    source.unlink()
    return missing


"""Optional metadata must not interrupt the mandatory Bayer writer."""
class ExposureOutput:
    def __init__(self, path):
        self.path = path
        self.file = None
        self.error = None

    def __enter__(self):
        try:
            self.file = self.path.open('x')
        except OSError as exc:
            self.error = str(exc)
        return self

    def write(self, row):
        if self.error is None:
            try:
                self.file.write(row)
            except OSError as exc:
                self.error = str(exc)

    def __exit__(self, *_):
        if self.file is not None:
            try:
                self.file.close()
            except OSError as exc:
                self.error = str(exc)


def exposure_receipt(folder, disk_log, first, last, pairs, error):
    if error is None:
        try:
            missing = finalize(folder, disk_log, first, last, pairs)
            return dict(exposure_status='partial' if missing else 'ok', exposure_missing_pairs=missing)
        except (OSError, ValueError) as exc:
            error = str(exc)
    return dict(exposure_status='unavailable', exposure_missing_pairs=pairs, exposure_error=error)
