using System;
using System.Collections.Generic;
using System.IO;
using System.Text;

internal sealed class DesktopLanguage
{
    private readonly string path = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
                                                "OpenX6", "ui-language");
    public bool IsChinese { get; private set; }

    public DesktopLanguage()
    {
        try { IsChinese = File.ReadAllText(path).Trim() == "zh"; }
        catch (IOException) { }
        catch (UnauthorizedAccessException) { }
    }

    public void Save(string code)
    {
        Directory.CreateDirectory(Path.GetDirectoryName(path));
        string temp = path + "." + Guid.NewGuid().ToString("N");
        try {
            File.WriteAllText(temp, code, new UTF8Encoding(false));
            if (File.Exists(path)) File.Replace(temp, path, null);
            else File.Move(temp, path);
            IsChinese = code == "zh";
        } finally { if (File.Exists(temp)) File.Delete(temp); }
    }

    public string Text(string message)
    {
        foreach (var entry in WorkerMessages) {
            if (message == entry.Key || message.StartsWith(entry.Key + " ", StringComparison.Ordinal)
                                     || (entry.Key.EndsWith(" ", StringComparison.Ordinal) && message.StartsWith(entry.Key, StringComparison.Ordinal)))
                return Text(entry.Value) + message.Substring(entry.Key.Length);
        }
        string translated;
        if (message.StartsWith("SD readback verified.", StringComparison.Ordinal) || message.StartsWith("SD 卡读回校验通过", StringComparison.Ordinal)) {
            foreach (var entry in Chinese) {
                if (!entry.Key.StartsWith("SD readback verified.", StringComparison.Ordinal)) continue;
                foreach (var prefix in new [] { entry.Key + " ", entry.Value + " " })
                    if (message.StartsWith(prefix, StringComparison.Ordinal))
                        return Text(entry.Key) + " " + Text(message.Substring(prefix.Length));
            }
        }
        if (!IsChinese) {
            foreach (var entry in Chinese) if (entry.Value == message) return entry.Key;
        }
        if (message.StartsWith("Update to ", StringComparison.Ordinal))
            return IsChinese ? "更新至 " + message.Substring(10) : message;
        return IsChinese && Chinese.TryGetValue(message, out translated) ? translated : message;
    }

    private static readonly Dictionary<string, string> WorkerMessages = new Dictionary<string, string> {
        { "5/6 · Completing and verifying package / 写入并校验封装", "5/6 · Completing and verifying package" },
        { "5/6 · Compressing system image / 压缩系统镜像", "5/6 · Compressing system image" },
        { "Missing required X6 module / 缺少必需模块: ", "Missing required X6 module: " },
        { "Official X6 1.3.59 recognized. Select official 1.2.36 to build. / 已识别官方 X6 1.3.59。此版本仅支持从官方 1.2.36 构建，请重新选择。", "Official X6 1.3.59 recognized. Select official 1.2.36 to build." },
        { "Unknown official firmware: whole-file SHA-256 rejected / 未识别的固件，完整文件摘要不匹配", "Unknown official firmware: whole-file SHA-256 rejected" },
        { "1/6 · 正在校验官方固件和三个模块", "1/6 · Verifying official firmware" },
        { "2/6 · 正在从官方模块重建系统镜像", "2/6 · Reconstructing system image" },
        { "3/6 · 正在使用本机 Holder 编译相机更新验证器", "3/6 · Building components" },
        { "4/6 · 正在逐文件读回镜像并编译安装保护程序", "4/6 · Building and verifying filesystem" },
        { "5/6 · 正在封装并读回固件", "5/6 · Packing firmware" },
        { "6/6 · 正在签名并验证最终固件", "6/6 · Signing and verifying" },
    };

    private static readonly Dictionary<string, string> Chinese = new Dictionary<string, string> {
        { "1/6 · Verifying official firmware", "1/6 · 校验官方固件" },
        { "2/6 · Reconstructing system image", "2/6 · 重建系统镜像" },
        { "3/6 · Building components", "3/6 · 编译组件" },
        { "4/6 · Building and verifying filesystem", "4/6 · 构建并校验文件系统" },
        { "5/6 · Packing firmware", "5/6 · 正在封装固件" },
        { "6/6 · Signing and verifying", "6/6 · 签名并验签" },
        { "5/6 · Compressing system image", "5/6 · 压缩系统镜像" },
        { "5/6 · Completing and verifying package", "5/6 · 写入并校验封装" },
        { "Unknown official firmware: whole-file SHA-256 rejected", "未识别的固件，完整文件摘要不匹配" },
        { "Official X6 1.3.59 recognized. Select official 1.2.36 to build.", "已识别官方 X6 1.3.59。此版本仅支持从官方 1.2.36 构建，请重新选择。" },
        { "Missing required X6 module: ", "缺少必需模块：" },
        { "Cancelled. Nothing was sent.", "已取消，未发送文件。" },
        { "Keep the card and power connected. If an update is in progress, let it finish. If the camera stays idle, restart normally with the card inserted. At a charging screen press Power once.", "请保持插卡和供电，等待安装完成。若相机仍处于空闲状态，请留卡正常重启。若进入充电界面，短按一次电源键。" },
        { "SD readback verified. A card scan was requested. Watch the camera for installation.", "SD 卡读回校验通过，已请求扫卡，请留意机身升级提示。" },
        { "SD readback verified. Scan not confirmed. Check the camera before retrying.", "SD 卡读回校验通过，尚未确认扫卡。请先检查相机状态再重试。" },
        { "SD readback verified. The file is ready on the camera SD card.", "SD 卡读回校验通过，成品已写入机内 SD 卡。" },
        { "20 GiB free space required on output volume.", "输出磁盘至少需要 20 GiB 可用空间。" },
        { "Choose an output folder without an existing OpenX6 SD Build.", "请选择一个尚无 OpenX6 SD Build 子目录的输出位置。" },
        { "Expected only the three verified X6 modules.", "目录中应只包含三个已校验的 X6 模块。" },
        { "Build SD package", "生成卡刷包" },
        { "Build input", "构建材料" },
        { "Cancel export", "取消导出" },
        { "Canceled", "已取消" },
        { "Canceling export", "正在取消导出" },
        { "Check for updates…", "检查更新…" },
        { "Checking firmware", "正在校验官方固件" },
        { "Checking…", "正在检查更新" },
        { "Choose", "选择" },
        { "Choose saved module folder…", "选择已保存的模块目录…" },
        { "Deploy to SD via Wi-Fi", "通过 Wi-Fi 部署到 SD 卡" },
        { "Downloading and verifying", "正在下载并校验桌面更新" },
        { "File", "文件" },
        { "First use: connect to X6 Wi-Fi to unpack", "首次使用请连接相机热点解包" },
        { "For Wi-Fi deployment, insert an SD card with no update files.", "如需通过 Wi-Fi 部署，请先在相机内插入一张没有升级包的 SD 卡。" },
        { "Language", "语言" },
        { "Looking for your X6", "正在查找相机" },
        { "Missing verified build receipt", "未收到构建验签结果" },
        { "Modules verified", "三个模块已校验，可以构建" },
        { "Official X6 firmware", "官方固件" },
        { "Open cache folder", "打开缓存目录" },
        { "Open output folder", "打开固件所在目录" },
        { "OpenX6 · SD Card Builder", "OpenX6 · 卡刷包构建工具" },
        { "Operation failed", "操作失败" },
        { "Preparing build", "正在准备构建" },
        { "Retry update check", "重试检查更新" },
        { "Saved on this PC · ready for offline builds", "已找到本机缓存，下次会自动使用" },
        { "Select a local build output folder", "选择电脑上的输出位置" },
        { "Select official X6 1.2.36 firmware", "选择官方固件" },
        { "Select the folder containing the three verified modules", "选择已保存的模块目录" },
        { "Selected module folder", "已选择模块目录" },
        { "Unpack on my X6", "在我的 X6 上解包" },
        { "Up to date", "已是最新版" },
        { "Update to ", "更新至 " },
        { "Verified X6 1.2.36", "已验证 X6 1.2.36" },
        { "Verified local copy", "已在电脑上保存并校验" },
    };
}
