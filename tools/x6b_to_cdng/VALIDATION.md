# Validation

Local validation: 2026-11-21, macOS arm64, Python 3.13, NumPy 2.2.6,
Numba 0.64.0, tifffile 2026.11.16, rawpy 0.25.1 / LibRaw.

- All eight regression test groups passed. They cover lossless reconstruction of
  all 16,384 code values, both single-eye predictor layouts, both generations of
  50/60 tables, malformed data, header bounds, record order, missing frames,
  source preservation, cancellation and failure receipts.
- Two actual dual-eye reference frames (30 and 60 fps) and four generated 50/60
  vectors decoded to exactly the reference decoder's SHA-256 values.
- An actual X6S8B02 7680² frame matched the C reference decoder byte for byte;
  all 24 valid MIPI RAW10 stripe CRCs matched.
- Actual 3840² and 7680² DNG outputs were independently read by both tifffile
  and LibRaw. Their Bayer values matched the decoded arrays exactly, including
  black/white levels of 64/15408 and 64/1023 respectively.
- The Tk interface was opened on macOS and used to convert a generated 50 fps
  input. It completed and produced both eye sequences and the mapping/report.
- Only generated test imagery is committed. Actual recordings and personal
  calibration files are not part of the repository.

The GitHub workflow runs the regression suite on macOS, Windows and Linux.
Consult its result for the specific commit. Local GUI acceptance above is for
macOS; Windows/Linux GUI interaction and grading-application timeline/audio
import have not been claimed as tested. DNG decoding is distinct from an editor's
interpretation of image sequences and sidecar clocks.
