# X6 hotspot command client

Join your own X6 hotspot, run pair, confirm the serial on the camera and allow the connection. Probe checks the UID returned by the camera.

```sh
python3 x6_root.py pair
```

```sh
python3 x6_root.py probe
```

Save the following three lines as command.txt, then run the last command.

```sh
p=$(/bin/busybox mktemp /tmp/x6-check.XXXXXX) || exit 1
trap 'rm -f "$p"' EXIT
printf 'owner-check\n' > "$p" && [ "$(cat "$p")" = owner-check ] && rm -f "$p" && printf 'write/read/delete OK\n'
```

```sh
python3 x6_root.py run --command-file command.txt
```
