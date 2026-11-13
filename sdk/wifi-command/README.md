# X6 热点命令客户端

电脑连接自己的 X6 热点后运行配对命令。按提示核对机身序列号，在相机上允许连接；probe 将检查命令回传的 uid。

```sh
python3 x6_root.py pair
```

```sh
python3 x6_root.py probe
```

将以下三行存为 command.txt，随后执行最后一条命令。

```sh
p=$(/bin/busybox mktemp /tmp/x6-check.XXXXXX) || exit 1
trap 'rm -f "$p"' EXIT
printf 'owner-check\n' > "$p" && [ "$(cat "$p")" = owner-check ] && rm -f "$p" && printf 'write/read/delete OK\n'
```

```sh
python3 x6_root.py run --command-file command.txt
```
