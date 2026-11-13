# 在 Windows 11 x64 验收 field_budget

准备完整的开发者验收 ZIP、官方加密的 X6 **1.2.36** 固件和自己持有的 **机内导出模块目录**。将 ZIP 解压到本机磁盘。固件、材料和解压目录均可放在带空格或中文名称的本地盘目录。系统盘建议预留 30 GiB。

## 1. 确认 WSL 2

在 PowerShell 执行 `wsl --status` 和 `wsl --version`。首次启用 WSL 时，可在管理员 PowerShell 执行 `wsl --install --no-distribution`，按系统提示重启。WSL 就绪后，默认测试可离线完成。验收会创建专用发行版 `OpenX6-DevQA`，重复运行会核对其版本并创建新的输出目录。

## 2. 跑一次离线构建

在解压后含有 `Start-Test.ps1`、`source.tar.gz` 和 `OpenX6-runtime.tar` 的目录打开 PowerShell，执行：

```powershell
powershell.exe -NoProfile -STA -ExecutionPolicy RemoteSigned -File .\Start-Test.ps1
```

本轮请从普通 Windows PowerShell 5.1 直接执行上面的原脚本，保持启动前的默认编码。脚本会显示标题为 `OpenX6 Developer Acceptance` 的窗口，依次打开固件、材料两个属于该窗口的选择框。桌面自动化可按窗口标题及所属进程定位。也可先分别在第一框、第二框点取消，确认窗口关闭且终端返回，再正式运行。

请将固件放在同时含中文和空格的本地路径。终端的 `WSL UTF-8 path OK` 应紧跟选中固件对应的完整 WSL 路径，中文字符逐字可读。脚本会将 WSL 实际返回的路径与预期路径逐字比较。反馈时注明直接启动方式、两个选择框的所属窗口和中文路径显示结果。

若 Windows 提示下载的脚本被阻止，核对 ZIP 的 SHA-256 后可在 ZIP 的“属性”中解除锁定，再解压。依次在 Windows 的两个文件选择框选官方固件、机内导出模块目录。脚本会核对运行环境、测试源码和示例的摘要，再以 WSL 普通用户预检模块，运行 240 Mbps 示例，生成锁定候选并从镜像读回。每轮生成独立的临时测试身份。材料的私有副本和测试身份会在测试正常结束或返回错误时清理。

**通过标准**是末行显示 `PASS` 和 `readback.json` 的路径。可以在资源管理器打开：

```text
\\wsl.localhost\OpenX6-DevQA\home\openx6\OpenX6-DevQA
```

打开本次 `run-…` 目录。核对 `readback.json` 的 `modules` 只有 `field_budget`，`install_allowed` 为 `false`。`output/OpenX6 Build/RESULT.json` 的 `installer_kind` 应为 `guard-locked`。同一轮的 `candidate_sha256` 应等于 `RESULT.json` 的 `sha256`。可把候选文件拖入 PowerShell 的 `Get-FileHash -Algorithm SHA256 -LiteralPath` 命令来独立核验。每轮有自己的持有者身份，应与本轮收据比较。

## 3. 验收 Windows 上的开发者窗口

上一步使用原生 Windows 文件选择框和 WSL 构建器。要在 Windows 桌面操作与 Mac 相同的 Tk 开发者窗口，需用 WSLg，并在该测试发行版另装 Debian 的 Tk 包。这一步安装依赖时可能需要网络。默认离线构建的结果和 WSLg 界面的结果分别记录。

```powershell
wsl -d OpenX6-DevQA -u root -- apt-get update
wsl -d OpenX6-DevQA -u root -- apt-get install -y python3-tk
powershell.exe -NoProfile -STA -ExecutionPolicy RemoteSigned -File .\Start-Test.ps1 -Gui
```

1. 在两个 Windows 文件选择框选好输入。终端随即列出本轮 GUI 需要的五个路径。保持终端开启。
   核对 `Official firmware` 行与 `WSL UTF-8 path OK` 行中的中文路径完全一致，直接使用终端显示的文字。
2. 在开发者窗口勾选 **Full OpenX6 build**，点 **Add JSON**，选终端列出的 `field_budget/module.json`。点 **Check modules**，应显示 `field_budget` 通过预检。
3. 在三行 **Choose** 中分别选择终端列出的官方固件、私有材料副本和 `owner-public.json`。固件校验完成后显示 `Verified X6 1.2.36`。
4. 点 **Build locked candidate**，选终端列出的 `output` 父目录。等到窗口显示 `Menu readback OK` 和 `field_budget`，再关闭窗口。终端会再独立读回一次，显示 `PASS`。

可额外用 缺少一个文件的模块目录副本做一次反例，预期提示 `模块目录校验失败`。如需反馈，提供所用模式、末条状态、`wsl --version` 和本轮 `readback.json` 即可。输入材料和身份由持有者自己保存。
