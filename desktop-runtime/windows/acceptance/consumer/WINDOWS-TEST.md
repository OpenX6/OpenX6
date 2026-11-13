# Windows 11 x64 消费者版升级验收

目标是直接双击旧版和新版的 `OpenX6.exe`，在同一个 Windows 账户、同一个 WSL 发行版中各构建一次，再用升级前的公钥核对两份真实卡刷包。测试在电脑的本地盘完成，到读回签名为止。

## 准备

1. 用普通 Windows 账户，确认 `wsl --status` 正常。优先使用专门的 QA 账户，避免把已有设备的真实身份用于测试。已有 `OpenX6-DevQA` 可保留，本测试使用 `OpenX6-Builder-v1`。两个 EXE 从始至终用同一个账户运行。
2. 在共享目录对照此轮的 `SHA256SUMS`。将 `Previous-Consumer.zip`、`OpenX6-Consumer-Upgrade.zip` 和 `verify_holder.py` 复制到本地，分别完整解压两份 ZIP 到不同目录。预留约 35 GiB 可用空间，两份卡刷包合计约 3.3 GiB。
3. 建议建 `C:\OpenX6QA`，将校验脚本放在此处。准备三个空目录 `C:\OpenX6QA\旧版 output`、`C:\OpenX6QA\新版 output`、`C:\OpenX6QA\receipts`。在本地盘准备官方 1.2.36 原文件和提供给本地验收的 机内导出模块目录。官方包 SHA-256 为 `2c0b9685f2861455d9f248bf2b8897b24c8fba05dd79662deee0796b0dbd6853`。
4. 两轮均用资源管理器双击 EXE，以其原生选择框选文件及目录。请记录窗口实际表现。同一台电脑已有原身份可直接复用。若已注册的消费者环境异常，请保留现场并回报；切勿注销或重置发行版来制造通过结果。

## 旧版先构建

打开从 `Previous-Consumer.zip` 解出的 EXE，选官方包、材料和 `旧版 output`。直到控制台显示 `Verified`、输出目录中出现 `OpenX6 SD Build\Insta360X6FW.bin`，按 Enter 退出。在 Windows PowerShell 执行以下两行。第一行保存升级前的五项公钥指纹，第二行记录当前 WSL 注册 ID。

```powershell
wsl.exe -d OpenX6-Builder-v1 -u openx6 --exec /usr/bin/python3 -IB /mnt/c/OpenX6QA/verify_holder.py snapshot /mnt/c/OpenX6QA/receipts/before.json
(Get-ChildItem 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Lxss' | Where-Object { $_.GetValue('DistributionName') -eq 'OpenX6-Builder-v1' }).PSChildName
```

确认出现 `PASS snapshot`。如 WSL 首次安装需要 Windows 重启，先完成 WSL 设置再打开旧 EXE。

## 直接打开新版

1. 双击从新版 ZIP 解出的 `OpenX6.exe`。可先在第一个选择框取消，确认没有输出。再打开一次，选相同官方包、材料和 `新版 output`。不移动 Holder 或设置额外的窗口宿主。确认程序显示 `Verified`，输出包在新目录，旧包仍在原处。
2. 重复上面的注册 ID 查询，两次应相同。在 PowerShell 运行下面的命令。按示例创建了目录时，可原样使用。

```powershell
wsl.exe -d OpenX6-Builder-v1 -u openx6 --exec /usr/bin/python3 -IB /mnt/c/OpenX6QA/verify_holder.py compare /mnt/c/OpenX6QA/receipts/before.json '/mnt/c/OpenX6QA/旧版 output/OpenX6 SD Build' '/mnt/c/OpenX6QA/新版 output/OpenX6 SD Build' /mnt/c/OpenX6QA/receipts/after.json
```

3. 应显示 `PASS compare`。校验脚本从每份实际 MWFI 包的模块表确定 FwCert 位置，重算整文件和签名覆盖内容的 SHA-256，用升级前已固定的维护公钥验签，再用另一把公钥确认拒绝。它还检查包内安装守卫中的五项公钥与升级前记录一致。新旧包的整文件摘要允许不同。
4. 校验损坏反例。另建 `C:\OpenX6QA\broken`，只将**新版** EXE 复制到其中，在该目录新建内容任意的短文件 `OpenX6-app.zip`。双击这个 EXE，选择有效输入和一个新的空输出目录。应看到程序摘要错误，输出目录应没有卡刷包。原来的新版解压目录继续可用。
5. 回执写明 Windows/WSL 版本、是否直接双击两代原 EXE、各选择框和中文路径是否可用、注册 ID 是否相同、两次构建及损坏反例是 PASS 还是 FAIL。仅回传简报、`before.json`、`after.json`。两份固件成品、机内导出模块目录和整个 Holder 留在本机；若失败，给出发生步骤和原始错误文本即可。
