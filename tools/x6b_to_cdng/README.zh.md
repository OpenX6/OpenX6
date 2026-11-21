# X6B 转 CinemaDNG

[English](README.md)

只需 Python 源码／脚本，无需编译应用、连接相机或提供固件密钥。
支持 OpenX6 **1.1 已发布**的 RAW Video：

| 档位 | 输出 |
| --- | --- |
| 双眼 3840×3840，30 fps | 无损还原原始 14-bit 码值 |
| 双眼 3840×3840，50／60 fps | 按各自量化表还原码值，兼容新旧格式；不能恢复拍摄时已丢弃的信息 |
| 单眼 7680×7680，30 fps | 无损还原原始 10-bit 码值，支持 X6S8B01／X6S8B02 |

工具根据文件头选择解码器，不按文件名猜测。不做去马赛克、增亮、镜像、
裁切、防抖或拼接，也不生成 JPG。每眼单独输出一组 DNG 序列。

## 安装及运行

安装 64 位 **Python 3.11–3.13**。Mac／Linux 已安装 Git 时，可以只取工具目录：

```sh
git clone --depth 1 --filter=blob:none --sparse https://github.com/OpenX6/OpenX6.git
cd OpenX6
git sparse-checkout set tools/x6b_to_cdng
cd tools/x6b_to_cdng
```

Windows 不接受仓库内部分无关的固件源文件名，请只导出本工具、不检出固件树
（PowerShell）：

```powershell
git clone --depth 1 --no-checkout https://github.com/OpenX6/OpenX6.git
cd OpenX6
git archive --format=zip --output=converter-source.zip --prefix=tools/x6b_to_cdng/ HEAD:tools/x6b_to_cdng
Expand-Archive converter-source.zip -DestinationPath .
cd tools/x6b_to_cdng
```

或者下载仓库 ZIP，只提取其中的 `tools/x6b_to_cdng` 文件夹。请保留该目录的全部内容，
包括两个 JSON 文件。下面的命令均在这个工具目录内执行。

Mac／Linux：

```sh
python3 -m venv .venv
.venv/bin/python -m pip install --only-binary=:all: -r requirements.txt
.venv/bin/python gui.py
```

Windows PowerShell：

```powershell
py -3 -m venv .venv
.venv\Scripts\python.exe -m pip install --only-binary=:all: -r requirements.txt
.venv\Scripts\python.exe gui.py
```

依赖使用预编译安装包，不需要 C/C++ 工具链。首次解码时 Numba 会自动准备本地缓存，
可能等待数秒。文件夹选择界面使用 Tkinter；python.org 的 Mac／Windows Python
安装器包含它。Linux 可能需要安装 `python3-tk`。也可以直接使用命令行，无需 Tkinter。
这是独立转换脚本，不会修改 OpenX6 固件构建工具。

## 转换

先复制**整条已录完的 RAWVIDEO 文件夹**。录制、拷贝或修复尚未完成时不要转换。
界面中选择素材文件夹，再选择一个位于素材之外的**新输出文件夹**，点击 Convert。

命令行示例：

```sh
.venv/bin/python convert.py "/素材/RAWVIDEO-20261118-120000" "/输出/take01"

# 从已保存的第 100 帧开始，只转换两帧、CAM0 通道：
.venv/bin/python convert.py "/素材/RAWVIDEO-20261118-120000" "/输出/sample" \
  --start 100 --count 2 --eye CAM0 --gaps error
```

Windows 将上述 Python 路径换成 `.venv\Scripts\python.exe`。
也能直接传入 `.x6b`，工具会从其所在文件夹读取 sidecar。
`--start` 从 0 起算，`--count` 统计实际保存的帧，不包括补帧。

缺帧策略：

- `hold`（默认）：缺失的帧重复上一帧，尽量保持录制时长。清单会明确标出，**不是找回了缺帧**。
- `keep`：保留文件序号缺口。有些后期软件会在缺口处停止识别序列，需要按 CSV 导入。
- `error`：发现所选范围内缺帧就停止。

工具按请求号排序，再核对曝光时间戳。单眼以硬件帧号计算缺帧，双眼以请求号计算。
帧率根据实际曝光间隔在整数帧率和 1000/1001 帧率之间选取。只有一条记录时，
30／60 默认使用 30000/1001、60000/1001，50 使用整数 50。原始时间戳始终保留。
不补第一张保存帧之前或最后一张之后的画面。**音频要按 audio-info 和时钟 metadata 对齐**，
不能假定音频起点就是第一张 DNG。

## 产物与限制

- `CAM0/`、`CAM1/`：每眼独立的 `.dng` 序列。单眼只生成录下的那一眼。
- `metadata/`：原样复制同目录 JSON、CSV、TXT、AAC、WAV，不复制巨大的 `.x6b`。
- `frames.csv`：每张 DNG 对应的请求号、硬件帧号、曝光时间戳、时长和补帧标记。
- `conversion.json`：完成／失败／取消状态、格式、缺帧数量。

CAM0／CAM1 是传感器通道，**不能当成物理左／右眼**。缺少单眼通道 metadata 时使用
`SINGLE`，不猜测眼别。几何标定、陀螺仪、加速度计、时钟文件原样保留；本工具不负责
防抖、几何重映射、音画合成或播放。

DNG 使用无压缩 16-bit 容器，内部保留原来的 10／14-bit 数值，包含 Bayer 排列、
黑白电平、帧率，以及可获得的实际曝光时长。不会把最初的手动 ISO 当成每帧实测 ISO。
颜色矩阵和白平衡采用现有开发参考，**不代表这条片子的实测色彩标定**，后期可自行调白平衡。
它也不是 iLog 渲染结果。

输出比 X6B 大：每张 3840² DNG 约 29.5 MB，7680² 约 118 MB。
一分钟双眼 30／50／60 约需 106／177／212 GB；单眼 8K30 约需 212 GB，另加音频等文件。
首版未加入 DNG 输出压缩。

原素材只读，已有输出目录一律不覆盖。损坏或截断的记录会报错，不静默跳过。
取消或失败后已完成的 DNG 保留，并在清单中标为未完成；重试请换新目录。
单眼有有效 CRC 标记的条带会逐一检查；双眼格式没有相同的像素校验，不能把结构检查
通过当作损坏素材必然完好的证明。

测试方法和格式来源见 [英文说明](README.md#development--tests)，实际验收范围见
[VALIDATION.md](VALIDATION.md)。
