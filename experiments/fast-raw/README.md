# Fast RAW converter experiment

This local build targets the X6 1.2.36 16K Fast RAW format. It produces an AArch64 executable for validation. The desktop release builder continues to use its pinned component profile.

From the repository root, use Python and the OpenX6 firmware reader prerequisites, Zig and llvm-strip. Allow about 5 GiB of temporary space on the output volume.

```sh
python3 experiments/fast-raw/build.py --firmware /path/to/official.bin \
  --modules /path/to/module-folder /path/to/new/ps2native
```

The output directory must already exist. The build verifies the official package and extracts snapshot tuning from its pinned ISP library locally. The compiled output contains firmware-derived tuning and belongs in the user's local build. Source inputs shipped here contain the ABI adapter and the extraction recipe.

At conversion time the input shot supplies EXIF ISO and exposure, white balance and baseline exposure. Each output retains its shot's color matrices and DNG gain maps. The engine receives an empty optional lens-shading table and an identity working color matrix. The 17-entry chromatic aberration table, sharpness and gain-dependent noise-addback tuning come from the official snapshot descriptor.

## Validation boundary

Archived hardware comparisons on a matched shot found zero changed pixels on either eye when individually disabling the optional lens-shading input or substituting an identity working color matrix. Desktop replay verified the candidate's two-eye input mapping and complete DNG output against three archived references when supplied their recorded inference outputs. The candidate's combined engine inputs still need a native HTP comparison, including high gain and low light. The lux index used for native model selection has no verified source in these DNGs. The experiment requests the baseline lux band. EXIF ISO also rounds the original working gain. Keep these limitations separate from correctness of the DNG writer.
