"""Language preference and message catalog for desktop interfaces."""
import os
from pathlib import Path
import sys
import tempfile


def preference_path():
    if sys.platform == "win32":
        parent = Path(os.environ["LOCALAPPDATA"])
    elif sys.platform == "darwin":
        parent = Path.home() / "Library/Application Support"
    else:
        parent = Path(os.environ.get("XDG_CONFIG_HOME", Path.home() / ".config"))
    return parent / "OpenX6" / "ui-language"


def load_language(path):
    try:
        return "zh" if path.read_bytes().strip() == b"zh" else "en"
    except OSError:
        return "en"


def save_language(path, code):
    if code not in ("en", "zh"):
        raise ValueError("Unsupported language")
    path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(dir=path.parent, delete=False) as f:
        tmp = Path(f.name)
        f.write(code.encode("ascii"))
    try:
        tmp.replace(path)
    finally:
        tmp.unlink(missing_ok=True)


CHINESE = {
    'Camera authorization timed out.': '等待机身授权超时。',
    'Camera approval timed out': '等待机身授权超时。',
    'Connected camera changed.': '当前连接的相机已改变。',
    'Incomplete X6 module set.': 'X6 模块不完整。',
    'Module verification failed: ': '模块校验失败：',
    'Unsupported export version': '此固件版本不支持机内解包。',
    'Camera export needs 2 GiB of free space.': '机内导出需要电脑有 2 GiB 可用空间。',
    'Camera export timed out.': '机内导出超时。',
    'Camera export failed. Check camera power, Wi-Fi and firmware.': '机内导出失败。请检查相机供电、热点和固件版本。',
    'On-camera export requires an authorized stock 0.8.825 or 0.8.827 X6.': '机内导出需要原厂 0.8.825 或 0.8.827，并须在机身授权。',
    'Scan receipt timed out': '等待相机扫卡回执超时。',
    'Signed build receipt mismatch': '卡刷包签名回执不匹配。',
    'Expected a verified signed build': '需要已验签的卡刷包。',
    'Choose a parent without an existing OpenX6 Build directory.': '请选择没有同名构建目录的输出位置。',
    '20 GiB of local free space required.': '输出磁盘至少需要 20 GiB 可用空间。',
    'timed out': '连接超时。',
    'Truncated official firmware': '官方固件文件不完整。',
    'Official firmware size changed': '官方固件文件大小发生变化。',
    'Holder identity mismatch.': '本机签发身份不匹配。',
    'Holder folder must be private.': '签发身份目录只能由当前账户访问。',
    'SSH identity mismatch.': 'SSH 身份不匹配。',
    'ADB identity mismatch.': 'ADB 身份不匹配。',
    'Select the verified X6 module folder.': '请选择已校验的 X6 模块目录。',
    '1/6 · Verifying official firmware': '1/6 · 校验官方固件',
    '2/6 · Reconstructing system image': '2/6 · 重建系统镜像',
    '20 GiB free space required on output volume.': '输出磁盘至少需要 20 GiB 可用空间。',
    '3/6 · Building components': '3/6 · 编译组件',
    '4/6 · Building and verifying filesystem': '4/6 · 构建并校验文件系统',
    '5/6 · Completing and verifying package': '5/6 · 写入并校验封装',
    '5/6 · Compressing system image': '5/6 · 压缩系统镜像',
    '5/6 · Packing firmware': '5/6 · 正在封装固件',
    '6/6 · Signing and verifying': '6/6 · 签名并验签',
    '6/6 · Verifying output': '6/6 · 校验输出',
    'Add JSON': '添加',
    'Add a module to check': '请先添加模块',
    'Build SD package': '生成卡刷包',
    'Build in progress': '构建中',
    'Build input': '构建材料',
    'Build locked candidate': '生成锁定候选包',
    'Build on the computer': '选择电脑上的输出位置',
    'Camera changed': '相机已切换',
    'Cancel export': '取消导出',
    'Canceled': '已取消',
    'Cancelled. Nothing was sent.': '已取消，未发送文件。',
    'Check firmware': '校验',
    'Check for updates…': '检查更新…',
    'Check modules': '预检模块',
    'Check the serial on your X6. Keep power connected, the camera idle and the SD card inserted. Publishing the verified file can start installation. The copy on your computer is kept.': '请核对机身序列号，保持供电和插卡，相机须处于拍摄空闲状态。卡上校验完成后，相机可能开始安装。电脑上的原包会保留。',
    'Checking auto power-off and Wi-Fi auto-off': '正在设置并核对自动关机和 Wi-Fi 自动关闭',
    'Checking camera SD': '正在检查机内 SD 卡',
    'Checking firmware': '正在校验固件',
    'Checking local build': '正在核对本机成品',
    'Checking…': '正在检查',
    'Choose': '选择',
    'Choose an output folder without an existing OpenX6 SD Build.': '请选择一个尚无 OpenX6 SD Build 子目录的输出位置。',
    'Choose firmware and a build input': '请选择官方包并获取构建材料',
    'Choose official X6 1.2.36 first': '请先选择官方 X6 1.2.36 固件',
    'Choose saved module folder…': '选择已保存的模块目录…',
    'Choose the required files': '请补齐输入文件',
    'Confirm camera': '确认相机',
    'Connect to camera Wi-Fi and retry': '请连接目标相机热点后重试',
    'Connect your Mac to the X6 Wi-Fi and allow Local Network access.': '请连接相机热点，并允许 OpenX6 访问本地网络。',
    'Continue?': '继续吗？',
    'Could not verify {field}. Reconnect to the X6 Wi-Fi and retry.': '无法核对 {field}。请重新连接相机热点后重试，已生效的设置会保留。',
    'Create holder': '新建身份',
    'Deploy over Wi-Fi': '通过 Wi-Fi 部署',
    'Deploy this build? Installation may begin immediately after readback verification. Your local package will be kept.': '要部署这个卡刷包吗？读回校验后可能立即开始安装。电脑上的成品会保留。',
    'Deploy to SD via Wi-Fi': '通过 Wi-Fi 部署到 SD 卡',
    'Desktop update failed': '桌面更新失败',
    'Downloading and verifying OpenX6 Desktop': '正在下载并校验桌面更新',
    'Error: ': '错误: ',
    'Example: ': '示例: ',
    'Expected only the three verified X6 modules.': '目录中应只包含三个已校验的 X6 模块。',
    'Export canceled': '已取消',
    'File': '文件',
    'Final SD status uncertain. Check the camera before retrying.': '请先检查相机是否已进入升级，再决定是否重试。',
    'First use: connect to X6 Wi-Fi to unpack': '首次使用请连接相机热点解包',
    'For Wi-Fi deployment, insert an SD card with no update files.': '如需通过 Wi-Fi 部署，请先在相机内插入一张没有升级包的 SD 卡。',
    'Full OpenX6 build': '完整 OpenX6 构建',
    'Holder created': '身份已创建',
    'Holder profile': '持有者配置',
    'If still idle, restart with the card inserted. At the charging screen, press Power once.': '若仍空闲，可留卡正常重启。若停在充电界面，短按一次电源键开机。',
    'Keep the card and power connected. If an update is in progress, let it finish. If the camera stays idle, restart normally with the card inserted. At a charging screen press Power once.': '请保持插卡和供电，等待安装完成。若相机仍处于空闲状态，请留卡正常重启。若进入充电界面，短按一次电源键。',
    'Language': '语言',
    'Local build changed': '本机成品已变化，请重新构建',
    'Local build retained.': '已保留本机成品。',
    'Local build retained. Check the camera before retrying.': '已保留电脑上的成品，请先检查相机再重试。',
    'Looking for your X6': '正在查找相机',
    'Match the label on your camera. Use a powered, idle X6 with its SD card inserted and no firmware at the card root. Tap Allow on the camera if prompted.': '请核对机身序列号，接好电源，保持相机空闲，插入根目录没有升级包的 SD 卡。如有连接提示，请点允许。',
    'Match this serial suffix to your camera. Keep it on and connected. Tap Allow on the camera if prompted. OpenX6 will save three verified modules on this computer.': '请核对机身序列号后六位，保持相机开机并连接热点。如机身提示连接，请点允许。三个校验通过的模块将保存在本机，供以后构建使用。',
    'Menu readback OK (': '成品读回通过 (',
    'Missing required X6 module: ': '缺少必需模块：',
    'Modules': '模块',
    'Modules checked: ': '模块预检通过: ',
    'Modules verified': '三个模块已校验，可以构建',
    'Official X6 1.3.59 recognized. Select official 1.2.36 to build.': '已识别官方 X6 1.3.59。此版本仅支持从官方 1.2.36 构建，请重新选择。',
    'Official X6 firmware': '官方固件',
    'Official firmware': '官方包',
    'On-camera unpack requires stock 0.8.825 or 0.8.827.': '机内解包支持原厂 0.8.825 和 0.8.827。',
    'Open cache folder': '打开缓存目录',
    'Open output folder': '打开固件所在目录',
    'OpenX6 turns off auto power-off and Wi-Fi auto-off on this camera. These preferences stay in effect after the transfer.': 'OpenX6 将关闭相机自动关机和 Wi-Fi 自动关闭，传输结束后仍保留这两项设置。',
    'OpenX6 · Locked build': 'OpenX6 · 锁定候选包构建工具',
    'OpenX6 · SD Card Builder': 'OpenX6 · 卡刷包构建工具',
    'Output parent': '选择仓库外的输出父目录',
    'Reading menus and handlers from the candidate': '正在从成品读回菜单和模块…',
    'Receiving ': '正在接收 ',
    'Remove': '移除',
    'Retry update check': '重试检查更新',
    'SD readback verified. A card scan was requested. Watch the camera for installation.': 'SD 卡读回校验通过，已请求扫卡，请留意机身升级提示。',
    'SD readback verified. Scan not confirmed. Check the camera before retrying.': 'SD 卡读回校验通过，尚未确认扫卡。请先检查相机状态再重试。',
    'SD readback verified. The file is ready on the camera SD card.': 'SD 卡读回校验通过，成品已写入机内 SD 卡。',
    'Save holder outside the repository': '在仓库外保存身份',
    'Saved on this Mac · ready for offline builds': '已找到本机缓存，下次会自动使用',
    'Select official X6 firmware': '选择官方 X6 固件',
    'Select the three-module folder exported by your X6.': '请选择包含相机导出三个模块的目录。',
    'Select verified X6 module folder': '选择已校验的 X6 模块目录',
    'Selected module folder': '已选择模块目录',
    'Sending to camera SD': '正在通过 Wi-Fi 传到相机 SD 卡',
    'Serial ending': '序列号后六位',
    'Tap Allow on the camera': '请在相机上点允许',
    'Tap Allow on your X6': '请在相机上点允许',
    'Transfer timed out': 'Wi-Fi 传输超时',
    'Unknown official firmware: whole-file SHA-256 rejected': '未识别的固件，完整文件摘要不匹配',
    'Unpack on my X6': '在我的 X6 上解包',
    'Unpacking on camera': '正在机内解包，请保持相机开机和 Wi-Fi 连接',
    'Unsupported camera or firmware version': '相机机型或版本不受支持',
    'Unsupported camera updater': '此升级器不支持部署',
    'Up to date': '已是最新版',
    'Update to ': '更新至 ',
    'Verified ': '已验证 ',
    'Verified X6 1.2.36': '已验证 X6 1.2.36',
    'Verified cached modules': '已校验缓存，可离线构建',
    'Verified local copy: ': '已在电脑上保存并校验：',
    'Verified on SD.': '卡上已校验。',
    'Verified on SD. Scan requested. Keep power and card connected and watch the camera for the update.': '卡上已校验，已请求相机扫描。请保持供电和插卡，留意机身升级提示。',
    'Verified on SD. Scan unconfirmed.': '卡上已校验，扫描尚未确认。',
    'Verified: ': '已生成并校验: ',
    'Verifying camera SD': '正在从相机 SD 卡读回校验',
    'Waiting for camera': '正在等待相机',
    'Wi-Fi deployment': 'Wi-Fi 部署',
    'Working': '处理中…',
    'X6 module folder': '机内导出模块目录',
    'Your build remains in:': '电脑上的成品目录：',
}

# Progress and validation messages shared with the existing command-line tools.
WORKER_MESSAGES = {
    '1/6 · Verifying official firmware / 校验官方固件': '1/6 · Verifying official firmware',
    '2/6 · Reconstructing system image / 重建系统镜像': '2/6 · Reconstructing system image',
    '3/6 · Building components / 编译组件': '3/6 · Building components',
    '4/6 · Building and verifying filesystem / 构建并校验文件系统': '4/6 · Building and verifying filesystem',
    '5/6 · Completing and verifying package / 写入并校验封装': '5/6 · Completing and verifying package',
    '5/6 · Compressing system image / 压缩系统镜像': '5/6 · Compressing system image',
    '5/6 · Packing firmware / 正在封装固件': '5/6 · Packing firmware',
    '6/6 · Signing and verifying / 签名并验签': '6/6 · Signing and verifying',
    '6/6 · Verifying output / 校验输出': '6/6 · Verifying output',
    'Camera changed / 相机已切换': 'Camera changed',
    'Checking auto power-off and Wi-Fi auto-off / 正在设置并核对自动关机和 Wi-Fi 自动关闭': 'Checking auto power-off and Wi-Fi auto-off',
    'Checking camera SD / 正在检查机内 SD 卡': 'Checking camera SD',
    'Checking local build / 正在核对本机成品': 'Checking local build',
    'Export canceled / 已取消': 'Export canceled',
    'Export canceled / 已取消导出': 'Export canceled',
    'Final SD status uncertain. Check the camera before retrying. / 请先检查相机是否已进入升级，再决定是否重试。': 'Final SD status uncertain. Check the camera before retrying.',
    'Local build changed / 本机成品已变化，请重新构建': 'Local build changed',
    'Missing required X6 module / 缺少必需模块: ': 'Missing required X6 module: ',
    'Official X6 1.3.59 recognized. Select official 1.2.36 to build. / 已识别官方 X6 1.3.59。此版本仅支持从官方 1.2.36 构建，请重新选择。': 'Official X6 1.3.59 recognized. Select official 1.2.36 to build.',
    'Receiving / 正在接收 ': 'Receiving ',
    'Sending to camera SD / 正在通过 Wi-Fi 传到相机 SD 卡': 'Sending to camera SD',
    'Tap Allow on the camera / 请在相机上点允许': 'Tap Allow on the camera',
    'Tap Allow on your X6 / 请在相机上点允许': 'Tap Allow on your X6',
    'Transfer timed out / Wi-Fi 传输超时': 'Transfer timed out',
    'Unknown official firmware: whole-file SHA-256 rejected / 未识别的固件，完整文件摘要不匹配': 'Unknown official firmware: whole-file SHA-256 rejected',
    'Unpacking on camera / 正在机内解包，请保持相机开机和 Wi-Fi 连接': 'Unpacking on camera',
    'Unsupported camera or firmware version / 相机机型或版本不受支持': 'Unsupported camera or firmware version',
    'Unsupported camera updater / 此升级器不支持部署': 'Unsupported camera updater',
    'Verifying camera SD / 正在从相机 SD 卡读回校验': 'Verifying camera SD',
}

# Match only complete known messages or their documented dynamic suffixes.
def render(message, code):
    if isinstance(message, tuple):
        return ' '.join(render(part, code) for part in message)
    key = WORKER_MESSAGES.get(message, message)
    if key in CHINESE:
        return CHINESE[key] if code == 'zh' else key
    for raw, english in WORKER_MESSAGES.items():
        if message.startswith(raw) and (raw.endswith(' ') or message[len(raw):].startswith((' · ', ' '))):
            return render(english, code) + message[len(raw):]
    for prefix in ('Verified local copy: ', 'Verified: ', 'Verified ', 'Update to ', 'Receiving ',
                   'Module verification failed: ', 'Menu readback OK (', 'Modules checked: ', 'Example: ', 'Missing required X6 module: '):
        if message.startswith(prefix):
            return render(prefix, code) + message[len(prefix):]
    if message.startswith('Error: ') and len(message) > 7:
        return render('Error: ', code) + render(message[7:], code)
    if message.startswith('Could not verify ') and message.endswith('. Reconnect to the X6 Wi-Fi and retry.'):
        field = message[len('Could not verify '):].partition('.')[0]
        return render('Could not verify {field}. Reconnect to the X6 Wi-Fi and retry.', code).format(field=field)
    return message
