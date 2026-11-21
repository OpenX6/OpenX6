# X6B → CinemaDNG

[中文](README.zh.md)

A source-only converter for the RAW Video formats released with **OpenX6 1.1**.
No camera connection, firmware keys, application build, or C/C++ compiler is needed.
Python runs the tool; NumPy and Numba are installed as prebuilt wheels. Numba creates
a local decoder cache automatically on first use, which can take a few seconds.

| Recorded format | Accepted file headers | Output per captured eye |
| --- | --- | --- |
| Both lenses, 3840 × 3840, 30 fps | `L1024___` | Exact original 14-bit samples |
| Both lenses, 3840 × 3840, 50 fps | `X6M50ABC`, `X6M50AB2` | Reconstructed 14-bit samples from the recorded quantization table |
| Both lenses, 3840 × 3840, 60 fps | `X6QBLK05`, `X6QBLK0B` | Reconstructed 14-bit samples from the recorded quantization table |
| One lens, 7680 × 7680, 30 fps | `X6S8B01`, `X6S8B02` | Exact original 10-bit samples |

The source header selects the decoder and quantization table. No new quantization,
demosaic, exposure correction, mirror, crop, stabilization, or stitching is applied.
Values removed by the camera's 50/60 fps quantization cannot be recovered.

## Install

Use 64-bit **Python 3.11–3.13**. On macOS/Linux, with Git installed:

```sh
git clone --depth 1 --filter=blob:none --sparse https://github.com/OpenX6/OpenX6.git
cd OpenX6
git sparse-checkout set tools/x6b_to_cdng
cd tools/x6b_to_cdng
```

On Windows, some unrelated firmware source filenames are reserved by the OS.
Export just this tool without checking out the firmware tree (PowerShell):

```powershell
git clone --depth 1 --no-checkout https://github.com/OpenX6/OpenX6.git
cd OpenX6
git archive --format=zip --output=converter-source.zip --prefix=tools/x6b_to_cdng/ HEAD:tools/x6b_to_cdng
Expand-Archive converter-source.zip -DestinationPath .
cd tools/x6b_to_cdng
```

Alternatively, extract just `tools/x6b_to_cdng` from the repository ZIP. The entire
tool folder, including both JSON files, is needed. Run the following in that folder.

macOS / Linux:

```sh
python3 -m venv .venv
.venv/bin/python -m pip install --only-binary=:all: -r requirements.txt
.venv/bin/python gui.py
```

Windows PowerShell:

```powershell
py -3 -m venv .venv
.venv\Scripts\python.exe -m pip install --only-binary=:all: -r requirements.txt
.venv\Scripts\python.exe gui.py
```

The optional interface uses Tkinter, included in the standard python.org macOS and
Windows installers. Some Linux distributions need their `python3-tk` package.
The CLI does not need Tkinter. This is a separate script, not an update to the
OpenX6 firmware-building application.

## Convert

Copy the **whole completed RAWVIDEO folder** off the card. Do not convert a take
while it is still being recorded, copied, or repaired. In the interface, choose
the take and a **new output folder outside it**, then click Convert.

Command line (use the virtual environment's Python on your platform):

```sh
.venv/bin/python convert.py "/media/RAWVIDEO-20261118-120000" "/media/CDNG/take01"

# Extract two captured frames starting at captured index 100, one channel only:
.venv/bin/python convert.py "/media/RAWVIDEO-20261118-120000" "/media/CDNG/sample" \
  --start 100 --count 2 --eye CAM0 --gaps error
```

A `.x6b` path also works; the tool looks for sidecars in its parent folder.
`--start` and `--count` count **captured records after sorting**, not padded output
frames. Requests that were physically written out of order are sorted by request
ID and checked against exposure timestamps. Unknown formats, inconsistent metadata,
duplicate requests, invalid payloads and truncated records are rejected.

### Missing frames

- **`--gaps hold` (default):** repeat the previous DNG for missing frame IDs, keeping
  the internal recording cadence. Every repeat is marked `held_previous=True` in
  `frames.csv`. These are duplicates, not recovered exposures.
- **`--gaps keep`:** preserve holes in the numbered filenames. Some image-sequence
  applications stop at the first hole; use the mapping CSV when importing.
- **`--gaps error`:** refuse conversion if the selected range has missing frames.

Single-eye gaps use hardware frame numbers; dual-eye gaps use request IDs. Frame
rate is selected from the measured exposure cadence (integer or 1000/1001 rate).
For a one-record input, nominal 30/60 use 30000/1001 and 60000/1001; nominal 50 uses
50. Original nanosecond timestamps remain authoritative. Frames before the first
stored exposure or after the last are not invented. Separate audio can begin or
end outside this range: **use the audio timing sidecar to align it**, not filename
order or an assumption that audio starts at DNG frame zero.

## Output

```text
take01/
  CAM0/frame_00000000.dng
  CAM1/frame_00000000.dng       # absent for a single-eye take
  metadata/                   # original JSON/CSV/TXT and AAC/WAV sidecars
  frames.csv                  # output → request/hardware/timestamp/offset mapping
  conversion.json             # completion status, format and missing-frame counts
```

Channels are sensor identities, **not guaranteed physical left/right eyes**. A
single-eye take without `clip.json` uses a folder named `SINGLE`. No channel or
geometry is guessed. Geometry, gyro, accelerometer and clock data are copied
unchanged; stabilization, remapping, audio muxing and DNG playback are not provided.

Each DNG is an uncompressed 16-bit container holding the original 10- or 14-bit
integer values. It includes the Bayer pattern, black/white levels, a CinemaDNG
FrameRate tag, and measured ExposureTime when available. ISO is not fabricated
from the initial UI setting. The development reference color matrix and white
balance are used, **not a measured per-take color calibration**. Adjust white
balance in the grading application. This output is not an iLog rendering.

Storage is approximately **29.5 MB per 3840² eye/frame** or **118 MB per 7680²
frame**. A minute is roughly 106 GB for both eyes at 30 fps, 177 GB at 50 fps,
212 GB at 60 fps, or 212 GB for a single 8K eye at 30 fps, plus sidecars. No
additional output compression is included in this first script.

Sources are opened read-only. Existing output folders are refused. An interrupted
or failed job keeps completed DNGs and marks `conversion.json` accordingly; it
does not claim completion or repair the input. Choose a new folder to retry.
Single-eye RAW10 CRCs are checked wherever flagged in the file; dual-eye records
have no equivalent pixel CRC. Decoder validation is not proof that a damaged
source without a checksum is intact.

## Development / tests

```sh
.venv/bin/python -m pip install --only-binary=:all: tifffile
.venv/bin/python -m unittest discover -s tests -v
```

The tests include synthetic vectors for both versions of 50/60 fps, every 14-bit
code value in the lossless mapping, both single-eye block layouts, malformed
records, gaps, source preservation and independent TIFF reading. Test fixtures
contain generated pixels, not camera footage. See [VALIDATION.md](VALIDATION.md)
for the tested environment and interoperability limits.

Decoder/table provenance: the public source in
[`components/src/native/raw-video`](../../components/src/native/raw-video) at
commit `59bcfd5373f79ba577f2c7d9141d8097373cc5f0`. `levels.json` is the five
`levels` / `alternate_levels` arrays; `color-reference.json` is the published
runtime reference. The converter is licensed under the repository's [GPLv3](../../LICENSE).
