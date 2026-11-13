# Native Windows desktop

The Windows 11 x64 release starts at `OpenX6.exe`. It verifies the runtime and starts the WinForms GUI in `OpenX6.Desktop.exe`. The GUI calls the launcher to inspect, unpack, build or deploy. The packager embeds the stable update channel in the launcher and writes its identity to `desktop-release.json` in the ZIP root. Use the release checkout’s `release/desktop-channel.json` when building the distribution. The native build worker uses the current Windows account’s Holder.

To rebuild the desktop layer, verify the release ZIP against its published checksum and extract it into a staging directory. Use a local copy of that directory for development.

1. For a GUI change, run `build_gui.cmd "C:\path to\staging"` on Windows 11 x64 with the .NET Framework 4.x compiler. The GUI sources are in `GUI/`.
2. For worker changes, copy edited workers into their matching staging paths. The GUI language catalog goes in both `Reader/openx6/ui_language.py` and `Native/Source/openx6/ui_language.py`. For a firmware change, first prepare and test the corresponding build assets in staging.
3. From this source directory run the following with Python 3 and host Go (1.27.3 for the first release). Choose a new ZIP path outside staging.

```sh
python3 package_windows.py --bundle /path/to/staging --launcher Launcher --out /path/to/output/OpenX6-Windows11-x64-native.zip
```

`Launcher/runtime.sha256` lists every runtime file relative to the ZIP root. Packaging computes hashes from staging, embeds the list in a fresh x64 launcher, then reads back the ZIP. For a new release, update the source manifest to the reviewed staged hashes in the same order, and update the release checksum pins after acceptance. Exact ZIP hashes also depend on ZIP metadata. [Desktop release checks](../../update/README.md)
