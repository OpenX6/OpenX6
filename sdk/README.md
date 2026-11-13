# Module author guide

[中文](README.zh.md)

**Developer preview.** This guide covers the module interfaces you can try today. The manifest and menu interfaces may change. See the [current scope and direction of the framework](../README.md#module-framework-preview).

## Create your first module with Python

From the repository root, run these commands with Python 3.11 or later. All you need for these steps is Python and this checkout. On Windows you can substitute `py -3` for `python3`.

```sh
python3 -B -m sdk init ../field_budget
python3 -B -m sdk check ../field_budget/module.json
python3 -B -m sdk.menu_preview ../field_budget/module.json --set field_budget.rate max
```

The new directory contains `module.json`, `budget.py` and the template’s Apache-2.0 `LICENSE`. Edit the camera menu in `module.json` and the storage estimate in `budget.py`. The preview should show **260 Mbps** for Max. To try 240, change the option label in the manifest and the value for `max` in the program. Run `check` again. It reports the changed source digest and shows the current SHA-256. Review your edits, copy that value into `program.sha256`, then repeat the check. Run your own calculation with `python3 -I -B ../field_budget/budget.py max`. At 240 Mbps the expected result is **100.58 GiB per hour**.

Pass several manifests to `check` to test them together. It checks manifest structure, selection conflicts, handler digests and Python 3.10 syntax without running module code. It defaults to the full OpenX6 profile. Use `--profile stock` for stock-component byte edits. In the developer desktop tool, select the matching profile, add the same manifests and click **Check modules** for the same check. Verifying original bytes, overlaps with built-in patches and native menu capacity requires a locked build from official 1.2.36.

Share the manifest, its sibling source, its license, and a sample input with its expected output. New plugins contributed to this repository use Apache-2.0. Independent third-party plugins retain the license terms that apply to their own code and dependencies. Another developer can use these steps to check and build it independently. Consumer SD builds currently include built-in modules only.

## Author a byte patch

Run `python3 -B module_plan.py --base examples/sample.txt --target example examples/demo.json` from the SDK directory for a synthetic example.

Copy a sample JSON and choose a unique module ID. Pin the exact SHA-256 of the target, list dependencies and conflicts, and specify offsets with expected and replacement bytes in lowercase hex. Replacements must preserve length. Select dependencies explicitly. The builder rejects overlapping edits, unexpected original bytes and wrong target hashes.

| Desktop profile | Example | Target used for the digest |
| --- | --- | --- |
| Default | [stock_debug_label.json](examples/stock_debug_label.json) | Official 1.2.36 media executable |
| Full OpenX6 build | [media_debug_label.json](examples/media_debug_label.json) | Same executable after built-in features |

Both change the ELF debug link to `ox6_mediad`. Inspect `release-inputs.json` to compare the selected manifest and output digests. Supply an original manifest, a synthetic test and instructions for inspecting the change. Keep firmware, credentials and locally generated outputs outside contributions. [Desktop walkthrough](../README.md)

## Prototype a settings page offline

Copy [menu_grid.json](examples/menu_grid.json). Give the module a unique ID and declare rows as toggles or choices. Run this from the SDK directory:

```sh
python3 -B menu_preview.py examples/menu_grid.json --set grid_guide.enabled on --set grid_guide.style Center
```

The preview prints the page, row labels, available choices and selected values. The grid example uses the preview-only schema and keeps selections in memory for this run. The `field_budget` example above uses the buildable native menu schema.

## Build a native setting

In the developer desktop tool, select **Full OpenX6 build**, choose the official 1.2.36 file, your module folder and holder profile, then add [stats_overlay.json](examples/stats_overlay.json). Build a locked candidate. Its menu path is **Mod Settings → Extensions → Capture statistics → Show recording stats**. The built-in Stats switch moves to this page and uses the same saved preference. Omitting the module gives the standard menu layout.

Copy the example to change the module ID, page title and switch label. Use printable ASCII for native labels. The first runtime binding, `stats.overlay.enabled`, accepts a boolean with a fresh-install default of false. The controller loads the owner's existing preference. Selecting two modules that claim the binding fails before firmware reconstruction. Additional bindings need a core implementation and tests. The grid example above remains a desktop prototype.

Run `python3 -B sdk/menu_preview.py sdk/examples/stats_overlay.json --set capture_stats.enabled on` from the repository root to try the label and value. After building, check `menu-build.json` for the generated GUI hash and `RESULT.json` for `install_allowed: false`.

From the repository root, independently reconstruct the menu from the finished candidate with the same build tools:

```sh
python3 -B -m sdk.verify_menu_candidate "/path/to/OpenX6 Build" sdk/examples/stats_overlay.json
```

## Add a module-owned choice

Copy [field_checklist.json](examples/field_checklist.json). Under **Mod Settings → Extensions → Field checklist**, choosing Vehicle shows a reminder to check the mount and safety tether. Each manifest defines one native radio setting with 2–8 choices. Each choice supplies its own note. Multiple modules can coexist.

Preview the behavior before selecting the JSON in a full developer build:
```sh
python3 -B sdk/menu_preview.py sdk/examples/field_checklist.json --set field_checklist.rig vehicle
```
Keep module, setting and option IDs stable across releases. Changing labels or reordering choices preserves preferences. Removing the selected option prompts the owner to select a replacement and preserves the file until then. Failed saves display an error and reload the stored selection.

Module code can call `sdk.choice.read_choice(directory, doc)` with the parsed manifest and a local fixture directory or the camera preference directory `/data/x6-owner-features/extensions-v1`. A fresh setting returns its declared default. The file `openx6-choice.<module-id>.<setting-id>.pref` contains the ASCII option ID without a newline.

When building with the Stats example, pass both menu manifests to `sdk.verify_menu_candidate` for readback.

Try an independent reader against a local copy of the preference directory:
```sh
python3 -B -m sdk.examples.read_checklist "/path/to/preference-directory"
```
An empty directory returns the Tripod reminder. The example loads the saved ID and applies the module’s own ID-to-note mapping.

## Run your own program from a menu choice

The [storage budget module](examples/recording_budget/module.json) packages a native choice and a sibling [budget.py](examples/recording_budget/budget.py). Selecting **Mod Settings → Extensions → Storage estimate → 260 Mbps** yields an estimated 108.96 GiB of video payload per hour. Recording settings are controlled separately.

1. Copy the example directory and include the [SDK license](LICENSE) as `LICENSE` in your copy. Choose stable IDs for the module, setting and options. Write a single Python 3.10 script using the camera’s available standard library. Its first argument is the selected option ID. Print one UTF-8 JSON object of at most 8192 bytes to stdout and exit with status 0. Try your own script with `python3 -I -B budget.py max`.
2. Run `shasum -a 256 budget.py` or `sha256sum budget.py` and put the digest in `program.sha256`. Update it after editing the source. The builder checks the sibling source and its syntax without running the module or installing dependencies.
3. In the developer desktop tool choose **Full OpenX6 build**, supply the firmware, module folder and holder profile, and add the example’s `module.json`. Build a locked candidate.
4. From the repository root, run the readback below. Include any other selected menu manifests. It verifies the native menu, program source, service files and startup links without running contributor code.

```sh
python3 -B -m sdk.verify_menu_candidate "/path/to/OpenX6 Build" sdk/examples/recording_budget/module.json
```

The core forwards only the validated option ID to a dedicated unprivileged worker. The worker reconciles the latest saved choice at startup and on change. Write idempotent handlers. Rapid choices may coalesce. Each invocation has a three-second deadline. Failures clear the result and retry. Read the example result from the holder environment at `/run/openx6-result-recording_budget/result.json`. For Max it contains `{"selection":"max","result":{"mbps":260,"video_gib_per_hour":108.96,"minutes_per_128_gib":70.5}}`.

The builder assigns vendor-policy labels to the module units and their startup links. Before the GUI starts, a root-only helper prepares the private choice directory and migrates only the filenames declared by the selected modules, including their atomic-save staging files. It preserves existing selections and refuses symbolic links, hard-linked preferences, incorrect ownership or unexpected labels. A worker receives the selected ID through a read-only mirror and writes its own result under a private dynamic UID.

On an offline ARM64 VM, the stock 1.2.36 systemd and MLS policy passed startup, native menu saves, handler execution and reboot with the last selection in both permissive and enforcing modes. Worker access to the original owner preferences and another private result directory was denied. The VM uses a generic kernel and supplies the boot prerequisites. Camera boot ordering, labels after mounting the camera overlays and touchscreen operation still require hardware testing.

The stock systemd build omits seccomp. A worker can create sockets inside its private network namespace. Review contributor code before building; the source digest pins the reviewed version.
