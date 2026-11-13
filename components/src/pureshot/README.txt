x6 PureShot batch - turn 120 MP passthrough DNGs into the camera's native processed DNGs, on the camera

What it reads   120 MP DNGs shot with insta.iis.binopureshot.passthrough=1 (unprocessed Quad Bayer,
                WhiteLevel 1023, level-0 strips from the fast-dng flag). Anything else is skipped/refused.
What it writes  <storage>/PureShot/<name>_processed.dng (beside DCIM, never inside it), same tags as the
                input except WhiteLevel 16383 / BlackLevel 0, i.e. the file the camera writes natively.
                Inputs are never modified or deleted; existing outputs are never overwritten.

Use (over SSH, root):
  T=/usr/lib/x6-pureshot
  python3 $T/x6_pureshot_batch.py groups              # list passthrough groups on internal storage / SD / SSD
  python3 $T/x6_pureshot_batch.py plan 1              # show what group 1 would produce
  python3 $T/x6_pureshot_batch.py run 1               # convert group 1 ("all", "1,3", "2-4" also work)
  options: --root DIR (one folder only), --out DIR (output folder)
  single file: $T/ps2native IN.dng OUT.dng

Behaviour       ~13-15 s per frame; peak RSS about 2 GB while a frame converts. Runs a frame only while the
                camera is idle (CaptureIdle + RecordIdle) and no storage is exported over USB; checks again
                before every frame and stops cleanly when the camera gets busy or the output disk is
                nearly full. Re-running skips frames already converted. Don't shoot while it runs.

Fidelity        With a capture's own vendor parameters the output is byte-identical to the camera's native
                DNG (bundle-003). From the DNG alone, which is what the batch does, 99.98% of pixels are
                identical and the rest differ by 1 DN. The engine's lux index is not stored in DNGs, so
                frames are processed as lux 241 (the camera's indoor value); ISO and exposure come from the
                DNG and drive the engine's own model choice as usual.

Installed       by the firmware in /usr/lib/x6-pureshot (read-only root fs). The camera page
                Mod Settings > 16K Fast RAW Processing converts IN PLACE instead (daemon x6-pureshot-processor).
Source          OpenX6 components/x6-current/src/pureshot (ps2native/, x6_pureshot_batch.py)
