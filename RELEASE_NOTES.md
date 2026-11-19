# OpenX6 Desktop

## English

> **You can install OpenX6 for the first time only on an X6 currently running stock 0.8.825 or 0.8.827.** Keep the camera on its early firmware until you install OpenX6. Use the official 1.2.36 download only as a build input on your computer. **Do not copy the official Insta360 firmware package to the SD card.**
>
> **After installing OpenX6, do not install official firmware through the Insta360 app or from an SD card.** There is currently no known reliable way to reinstall OpenX6 after returning to stock firmware. Use a compatible OpenX6 SD package for future updates.

The Mac and Windows desktop tools build these capture modes from the official X6 1.2.36 package.

### RAW Video

In the 360 shooting menu, select **RAW Video** and connect an external SSD. The format picker offers 3840 × 3840 per lens at 30, 50 or 60 fps with both lenses, and 7680 × 7680 lossless RAW10 at 30 fps from the selected front or back lens. In the latter mode the preview stops during recording. Setting shutter or ISO to Auto lets that value follow the light while a manually set value stays fixed. Each take stores its Bayer video in an `.x6b` file, with audio and separate timing, exposure, calibration and motion files for desktop processing.

Both lenses deliver 14-bit Bayer samples (0–16,383). The frame rate determines how finely the file preserves those values.

| Lenses recorded | Per-lens format | Compression and precision | Estimated total sustained write |
| --- | --- | --- | ---: |
| One (front or back) | 7680 × 7680 · 30 fps | 10-bit Bayer input, lossless at every code value. | 870 MB/s |
| Both | 3840 × 3840 · 30 fps | Lossless. All 16,384 possible input values decode exactly. | 731 MB/s |
| Both | 3840 × 3840 · 50 fps | Lossy. Quantization steps range from 4 to 128 codes below 13,472. Maximum error is 64 codes. | 690 MB/s |
| Both | 3840 × 3840 · 60 fps | Lossy. Steps range from 8 to 512 codes below 13,472. Maximum error is 256 codes. Smooth sky gradients can show banding. | 626 MB/s |

At 50 and 60 fps, every input code from 13,472 to 16,383 is preserved exactly. Among the dual-lens modes, 30 fps retains the most detail in tone. The write figures are the camera’s initial estimates in decimal MB/s. Actual throughput varies with the scene, so allow headroom when choosing a drive.

RAW Video now prepares its capture pipeline automatically when selected and restores the previous shooting settings on exit. Single-lens 8K RAW uses X6S8B02 lossless compression; desktop decoders must support this format. Storage throughput still limits recording in complex scenes. If audio finalization fails, the camera preserves valid RAW video and records the stop reason and audio error alongside the take.

**Exposure tools.** Enable **Show Shutter & ISO** in Mod Settings to see the current values during automatic exposure. The highlight warning marks clipping in the preview.

**SD updates.** After a successful installation, the camera can clean up the consumed package and accept another update, including a reinstall of the same version. Existing settings and Holder keys carry over.

**Known SD update issue.** If another process is using the card during preparation, the update can stop before flashing and the progress panel can disappear. Automatic retry may then stall. This release retains that behavior. After confirming the camera has returned to idle, retry with a spare empty card containing only the intended OpenX6 firmware. Formatting is optional and does not guarantee success. A package installed through the native update path may leave a hidden archive on the card.

### Panoramic video

| Menu choice | Combined width × height | Nominal frame rate |
| --- | ---: | ---: |
| 10K | 9728 × 4864 | 30 fps |
| 10K Crop | 9824 × 4912 | 30 fps |
| 7.4K | 7424 × 3712 | 60 fps |
| 7.4K Crop | 7424 × 3712 | 60 fps |

Standard modes use the full sensor width. Crop modes use pixel-to-pixel readout for a tighter view. The combined width is the sum of the two recorded lens tracks.

### FreeFrame video

Single-lens FreeFrame records **up to 7680 × 7680 at 30 fps in 10-bit iLog**.

| FreeFrame menu choice | Recorded width × height | Nominal frame rate |
| --- | ---: | ---: |
| 8K | 7680 × 7680 | 30 fps |
| 7.3K Full | 7296 × 7296 | 30 fps |
| 6.7K Full | 6656 × 6656 | 30 fps |
| 5K Crop | 4912 × 4912 | 24 / 25 / 30 fps |
| 6K Crop | 6000 × 3968 | 60 fps |
| 4K90 | 3840 × 3840 | 90 fps |
| 4K120 | 3840 × 3008 | 120 fps |

**Finding a FreeFrame mode.** In single-lens FreeFrame, tap the resolution and frame rate on the shooting screen, then tap **Ratio**. Select **1:1** for 8K, 7.3K Full, 6.7K Full, 5K Crop and 4K90. Select **4:3** for **4K120** or **6K Crop at 60 fps**. The table gives the recorded dimensions for each menu choice.

Crop modes and single-lens modes above 4K use the sensor’s small pixels. Expect more visible noise, especially in shadows or low light.

**Custom bitrate in 5K Crop and 6K Crop.** Both FreeFrame Crop modes apply Custom targets above 245 Mbps. Actual bitrate varies with the scene.

The original 6K square mode remains separately selectable at **6016 × 6016**. For square 8K30, turn on **Disable Video Proxy**. With the full-rate proxy, the camera exhausted recording buffers during a test. The HEVC encoder now requests output buffers of at least 24 MiB for 7296 × 7296 and 28 MiB for 7680 × 7680. In 7680 × 7680 FreeFrame, it uses eight input slots when the driver reports a minimum of seven and the original count is sixteen. Long-recording performance with this change still awaits camera testing.

### Higher video bitrates

**Up to 320 Mbps**, customizable in the camera menu. Usable bitrate depends on the mode and recording conditions.

| Recording mode | Max target | Custom target |
| --- | ---: | ---: |
| Panoramic video, Standard or Dolby Vision | 220 Mbps total | 1–320 Mbps total |
| Panoramic video, iLog | 280 Mbps total in 7.4K at 60 fps / 260 Mbps total in 7.4K Crop and other modes | 1–320 Mbps total |
| Single-lens video, including FreeFrame | 245 Mbps | 1–320 Mbps, up to 340 Mbps in square FreeFrame 4K60 |

In panoramic video the two lenses share the target. A single-lens recording gets the whole target. **Stats Overlay** shows measured bitrate, encoder input-buffer return time, frame timestamp gaps and SoC temperature. Tap the temperature line for sensor readings. High targets can still drop frames in demanding scenes. **Disable Video Proxy** and **Disable Recording Preview** work independently. In 89 supported FreeFrame and panoramic recording profiles, Disable Video Proxy skips creation of the encoded proxy and keeps the YUV stream required by the camera. If the alternate profile cannot load, the camera uses its original recording profile. Disable Recording Preview stops the live view five seconds into recording, keeps the stats visible, and restores live view when you stop.

**Choosing a recording bitrate.** Start with **Max**. It selects a preset for your recording mode and color profile, tuned for the highest bitrate that proved reasonably stable in testing. In **Stats Overlay**, **Return** is how long the encoder holds an input buffer. A sustained rise signals a backlog. **Gaps** counts missing frame intervals in the timestamps. Check both while recording, especially if you set **Custom** above Max. If Gaps keeps climbing in your scene, lower the bitrate.

For iLog video, the sharpness menu includes **Extra Low** and **Minimal**. In **Mod Settings → Video Noise Reduction**, choose Native, Medium, Low or Minimal. RAW photos have their own noise reduction setting. For video, start with **Video Noise Reduction = Native** and **Sharpness = Low**. Reducing noise reduction leaves more noise for the encoder to compress. Very low settings can make compression artifacts more visible in detailed or moving scenes. Judge changes on a recorded clip at your intended bitrate.

After installing this update, the camera asks you to confirm twice before handing a stock firmware update from the mobile app or SD card to the installer. Choose **Cancel** to keep OpenX6. Installing stock firmware removes OpenX6 and root access.

**Link Stereo AE / WB** links auto exposure and white balance between the two lenses for both photos and panoramic video, including 8K50.

**Fisheye preview, showing what the sensor captured natively.** The lower-left view menu offers Native, Linear and Fisheye in regular panoramic video. FreeFrame and regular panoramic video each remember your view choice across restarts. Fisheye framing follows the selected mode when recording starts or stops. In Flat and FreeFrame, disabling the front-lens preview mirror now preserves the selected framing without changing the recorded video. Returning to FreeFrame or regular single-lens video keeps 360° Audio when selected. FreeFrame shows the selected lens. In regular panoramic video, you can switch between the front and rear lenses to frame each view. This setting controls the on-camera preview.

### Panoramic Photo, SSD intervals and Fast RAW processing

For 120 MP stills, choose **RAW Only** for DNG output. **120 MP Sync** controls capture timing between the two lenses. **RAW Photo Noise Reduction** has four levels from Native through Medium and Low to Minimal.

Enable **Use External SSD** to record directly to a compatible USB-C exFAT drive. For interval photos, select **RAW Only**, open **Mod Settings → Interval Settings**, enable the matching **8K Fast RAW** or **16K Fast RAW** option, and select **Minimum**. The camera estimates a target from the shutter setting and the drive’s measured write speed. **Shutter Max** allows automatic exposures up to 2 s. **Custom** lets you enter your own interval.

**20× faster intervals.** At 1/100 s, 65 Fast RAW shots on SSD had a median start-to-start interval of **0.556 s**. The comparison uses one **11.296 s** interval between two native captures on SSD, including a 3 s timer after the first capture finished. The table measures **capture start to capture start** on a USB-C exFAT SSD with manual shutter. The 16K series used a 1 TB Genki SavePoint. Each range covers all gaps in that run.

| Fast RAW mode | Shutter | Photos saved | Median interval | Shortest–longest |
| --- | ---: | ---: | ---: | ---: |
| 8K / 30 MP, Minimum | 1/100 s | 22 | **0.355 s** | 0.310–0.435 s |
| 16K / 120 MP, Minimum | 1/100 s | 65 | **0.556 s** | 0.543–0.568 s |
| 16K / 120 MP, Minimum | 1/3 s | 62 | **0.874 s** | 0.873–0.876 s |

In separate speed tests with a 100 ms timer, 8K reached **0.33 s median** over 65 shots (shortest **0.29 s**). At 16K and 1/100 s, 82 shots averaged **0.466 s** (shortest **0.425 s**). Budget about **60 MB per 8K DNG** or **241 MB per 16K DNG**. Longer exposures and slower sustained writes increase the interval. For long unattended runs, allow some margin above Minimum.

**Ready-to-edit 16K RAW.** Select panoramic **Interval** photos, **16K (120 MP)**, **1:1** and **RAW Only**. Before starting the sequence, open **Mod Settings → Interval Settings**, turn **16K Fast RAW** off and set **Interval Timing → Minimum**. Press the shutter to start. The camera finishes PureShot processing as it shoots and saves standard Bayer DNGs ready to edit.

At a manual **1/100 s** on internal storage, four photos gave three capture-start intervals of **5.075, 4.516 and 4.550 s** (median **4.55 s**, about **0.22 photos/s**). Allow roughly **4.5–5 s per photo** at a short shutter. A separate three-photo SSD run with automatic exposure capped at 1/8 s produced files **4.46 and 4.56 s apart**, measured at save time. Longer exposures or slower writes can extend the interval. **Custom** accepts a target as low as **4.0 s** in this mode, but actual captures still wait for the preceding photo to finish.

**Why 16K Fast RAW needs processing.** To shorten the capture interval, the camera writes the 120 MP DNG with the sensor’s **Quad Bayer** pattern intact. Each 2 × 2 group samples the same colour. A RAW editor that reads those samples as ordinary Bayer can show a checkerboard or false colours, even if the DNG opens. After the shoot, the camera’s PureShot pipeline converts the samples to a standard Bayer pattern at the full **15520 × 7760** resolution. You can then edit the DNG in your usual RAW workflow. The 8K Fast RAW DNG (7776 × 3888) skips PureShot and needs no such conversion. In both modes the camera can begin the next capture while the preceding frame finishes writing.

**After a 16K Fast RAW shoot**, leave the SSD connected, open **Mod Settings → 16K Fast RAW Processing**, select the groups and tap **Start processing**. The camera verifies each result before replacing the original DNG in place. Copy the originals first if you want to keep the Quad Bayer files. A 34-frame batch averaged **5.7 s per photo** for this step.

**16K timelapse video** produces a video directly and has a separate **245 Mbps per lens** option. The sub-second figures above apply to interval DNG photos.

### Storage, audio and camera settings

Camera Info displays `1.2.36 OpenX6 1.1`. Single-lens video supports iLog and spatial audio. Wi-Fi idle timeout can be set to always on, 2, 5 or 10 minutes. The SD update screen shows verification and copy progress. Compatible reinstalls retain the camera’s settings and Holder keys. The unified update includes the MCU 1.2.0 application and skips that write when it is already current.

### Check your camera first

| What to check | Supported in this release |
| --- | --- |
| Desktop build input | Official X6 **1.2.36**. Unpack over Wi-Fi on stock **0.8.825** (camera-tested) or **0.8.827** (offline-tested). Later builds can use the verified modules saved on the computer. |
| First installation on a stock X6 | Factory **0.8.825** or **0.8.827**, with the matching updater and partition layout. Keep the camera on its factory version until you install OpenX6. Stock **1.2.36** and **1.3.59** cameras cannot install this SD update. |
| Updating an existing installation | An X6 already running a compatible modified build with SD maintenance updates enabled. Back up your `Holder` directory and use it again for future builds. |

A writable SD card can request one troubleshooting boot with `OPENX6-BOOT.TXT`. Choose `LEVEL1` to skip mods, `LEVEL2` for rescue, or `NORMAL` to cancel a queued request. The hotspot root command service on port 23456 is available in LEVEL2. Before marking a new slot successful, the camera verifies root locally through maintenance SSH or the hotspot service. Either can pass on its own. If both fail, the slot stays unmarked. The updater must also be idle.

### Get started

The desktop app starts in English. Use **Language → 简体中文** to switch. Your choice is saved for the next launch.

Open **OpenX6.app** on Apple Silicon or **OpenX6.exe** from the fully extracted native ZIP on Windows 11 x64. Select the official 1.2.36 package. To unpack it on a stock 0.8.825 or 0.8.827 X6, connect to its hotspot, choose **Unpack on my X6**, match the serial suffix and tap **Allow** on the camera if prompted. Keep the camera awake until the three modules have been verified. You can disconnect and build from the saved modules. The 0.8.827 updater passed offline tests. On-camera export on this version awaits verification.

With the official firmware and verified module folder selected, build the package. **Do not copy the official Insta360 firmware package to the SD card.** After OpenX6 verifies the build, copy only the generated `OpenX6 SD Build/Insta360X6FW.bin` to the root of the SD card. Safely eject the card and insert it into the camera. Keep the camera powered through the update.

The camera’s Wi-Fi hotspot accepts root shell commands from connected clients. Keep the hotspot password private.

**Validation.** On Windows 11 x64, the release ZIP passed native launch, language switching and a full build with an existing Holder. The resulting package passed independent signature verification. Fresh-camera installation of that build remains untested.

### Module framework: developer preview

**Try it today.** With Python and the repository, you can preview the recording budget example’s menu on your computer, choose a bitrate and run its handler to estimate storage per hour. To start your own module, run `python3 -m sdk init ../field_budget`, then `python3 -m sdk check ../field_budget/module.json`. In the developer tool you can build selected modules against official 1.2.36 and verify the menu, program and startup service by reading them back from the finished candidate. These candidates are install-locked for offline inspection. Installable SD packages currently include built-in features only. [SDK quickstart](sdk/README.md#create-your-first-module-with-python)

**Planned direction.** The goal is for authors to maintain their feature’s menu, code and tests, and for users to select compatible community modules alongside built-in features when building an installable SD package. The core would handle compatibility checks and package verification. The manifest format and menu interfaces may still change.

OpenX6 Core uses **GPL-3.0-only**. The module SDK and bundled examples use **Apache-2.0**.

For the full installation steps, see [the user guide](README.md).

## 中文

> **首次安装 OpenX6 只支持机身当前运行原厂 0.8.825 或 0.8.827 的 X6。** 首装前请保留机身的早期固件。官方 1.2.36 只用于在电脑上构建 OpenX6。**不要把 Insta360 官方固件包拷入 SD 卡。**
>
> **装好 OpenX6 后，请勿通过 Insta360 App 或 SD 卡刷回官方固件。** 刷回后，目前没有已知可靠的方法重新安装 OpenX6。后续更新请使用兼容的 OpenX6 卡刷包。

Mac 和 Windows 桌面工具均以官方 X6 1.2.36 包为输入，构建包含下列拍摄档位的卡刷包。

### RAW Video

连接外置 SSD 后，在 360 拍摄菜单选择 **RAW Video**。双眼档每眼 3840 × 3840，可选 30、50 或 60 fps。单眼档可选前镜头或后镜头，以 7680 × 7680、30 fps 无损记录 RAW10，开录后停止取景。快门和 ISO 可分别设为自动或手动，选自动的项目会在录制中跟随光线。每条素材的 Bayer 视频存为 `.x6b`，同目录保存音频、帧时间、曝光、标定和运动数据，供桌面后期使用。

双眼三档都读取 14-bit Bayer，每个像素的输入码值范围为 0–16383。帧率决定文件能保留多细的色阶。

| 镜头 | 每眼规格 | 压缩与精度 | 参考持续写速（整机合计） |
| --- | --- | --- | ---: |
| 单眼（前或后） | 7680 × 7680 · 30 fps | 10-bit Bayer 输入，全码值无损。 | 870 MB/s |
| 双眼 | 3840 × 3840 · 30 fps | 无损。输入的 16384 个可能码值都能逐值还原。 | 731 MB/s |
| 双眼 | 3840 × 3840 · 50 fps | 有损。低于 13472 时按亮度分段，量化步长为 4 至 128 个码值，最大误差 64。 | 690 MB/s |
| 双眼 | 3840 × 3840 · 60 fps | 有损。低于 13472 时按亮度分段，量化步长为 8 至 512 个码值，最大误差 256。天空等平滑渐变更容易出现色阶。 | 626 MB/s |

50 和 60 fps 都完整保留 13472–16383 的每一个输入码值。双眼录制时，如果后期要大幅调整天空等渐变区域，30 fps 保留的色阶最完整。表中为相机的初始写速估算，按十进制 MB/s 计算；实际数据量随场景变化，选盘时应为持续写入留出余量。

选 **30 Single Lens** 后，可用取景页镜头按钮选择录制的一眼。进入 RAW Video 时自动准备，退出时恢复原拍摄设置。后期请复制整条文件夹，单眼 8K 解码器需支持 X6S8B02。暗场或复杂场景可能超过盘的持续写速。若音频收尾失败，完整的 RAW 图像仍可独立封装，素材目录会记录停录原因及音频错误。

**曝光辅助。** 在 Mod Settings 打开 **Show Shutter & ISO**，自动曝光时也能看到当前快门和 ISO。开启高光报警后，取景页会标出达到裁切阈值的区域。

**SD 卡更新。** 安装成功后，相机会清理上次已用的安装包，可继续更新或重装同版本，并保留已有设置和 Holder 密钥。

**已知 SD 更新问题。** 准备阶段若有其他进程占卡，更新可能在刷写前停止，进度框随之消失，后续自动重试也可能停住。本版保留此行为。确认相机已回到空闲状态后，可用一张仅放目标 OpenX6 固件的空备用卡重试；格式化为可选操作，不能保证解决。走原生更新路径的包可能在卡上保留隐藏归档。

### 全景录像

| 菜单档位 | 双眼合计宽 × 高 | 标称帧率 |
| --- | ---: | ---: |
| 10K | 9728 × 4864 | 30 fps |
| 10K Crop | 9824 × 4912 | 30 fps |
| 7.4K | 7424 × 3712 | 60 fps |
| 7.4K Crop | 7424 × 3712 | 60 fps |

普通档使用传感器全宽，Crop 档逐像素读出，视角更窄。合计宽度是文件中两路镜头视频轨的宽度之和。

### FreeFrame 视频

单镜头最高可录 **7680 × 7680、30 fps、10-bit iLog**。

| FreeFrame 菜单档位 | 文件实际宽 × 高 | 标称帧率 |
| --- | ---: | ---: |
| 8K | 7680 × 7680 | 30 fps |
| 7.3K Full | 7296 × 7296 | 30 fps |
| 6.7K Full | 6656 × 6656 | 30 fps |
| 5K Crop | 4912 × 4912 | 24 / 25 / 30 fps |
| 6K Crop | 6000 × 3968 | 60 fps |
| 4K90 | 3840 × 3840 | 90 fps |
| 4K120 | 3840 × 3008 | 120 fps |

**如何找到 FreeFrame 档位。** 切到单镜头 FreeFrame，点拍摄页下方的分辨率和帧率，再点 **Ratio（画幅）**。选 **1:1** 可找到 8K、7.3K Full、6.7K Full、5K Crop 和 4K90。要拍 **4K120** 或 **6K Crop 60 fps**，选 **4:3**。上表列的是各菜单档位实际录出的宽和高。

各 Crop 档和超过 4K 的单镜头档位都使用传感器的小像素。阴影和暗光处的噪点会更明显，这是这些读出模式的正常表现。

**5K Crop 和 6K Crop 的 Custom 码率。** 这两个 FreeFrame 档位均支持高于 245 Mbps 的 Custom 目标，实际码率随场景变化。

原厂方形 6K 仍可单独选择，实际尺寸为 **6016 × 6016**。方形 8K30 建议开启 **Disable Video Proxy**，保留全帧率代理时实测出现过录像缓冲耗尽。HEVC 编码器现为 7296 × 7296 和 7680 × 7680 分别申请至少 24 MiB 和 28 MiB 的输出缓冲。7680 × 7680 FreeFrame 在驱动报告最少需要 7 个输入槽、原配置为 16 槽时使用 8 槽。这项改动的长录表现仍待实机验证。

### 提升录像码率

**录像码率最高 320 Mbps**，可在机身菜单中自定义。实际可用码率取决于档位和拍摄环境。

| 录像模式 | Max 目标码率 | Custom 自定义范围 |
| --- | ---: | ---: |
| 全景录像，标准色彩或 Dolby Vision | 双眼合计 220 Mbps | 双眼合计 1–320 Mbps |
| 全景录像，iLog | 7.4K 60 fps 双眼合计 280 Mbps，7.4K Crop 及其他档位为 260 Mbps | 双眼合计 1–320 Mbps |
| 单镜头录像，含 FreeFrame | 245 Mbps | 1–320 Mbps，方形 FreeFrame 4K60 最高 340 Mbps |

全景由两路视频分配总码率，单镜头可将整个目标码率用于一眼。**Stats Overlay** 实时显示实际码率、编码器归还输入缓冲的耗时、帧时间戳缺格和 SoC 温度，点按温度行可展开传感器读数。高码率在部分场景仍会掉帧。**Disable Video Proxy** 和 **Disable Recording Preview** 可分别开关。在 89 个支持的 FreeFrame 和全景录像配置中，关闭代理会跳过代理视频编码，并保留相机所需的 YUV 流。备用配置加载失败时会回到原录像配置。关闭录像预览则在开录约 5 秒后停止取景，统计信息继续显示，停录后恢复取景。

**录像码率怎么选。** 建议先选 **Max**。相机会按录像档位和色彩模式自动选用实测中能较稳定录制的最高预设码率。打开 **Stats Overlay**，观察 **Return**（编码器归还输入缓冲所用的时间）和 **Gaps**（按时间戳统计的缺帧数）。Return 持续升高说明画面开始积压；若 Gaps 持续增加，就降低码率。用 **Custom** 超过 Max 时尤其要留意，同一码率在不同光线、纹理和运动条件下表现可能不同。

iLog 录像的锐度菜单新增 **Extra Low** 和 **Minimal**。在 **Mod Settings → Video Noise Reduction** 中可选择 Native、Medium、Low 或 Minimal。RAW 照片的降噪单独设置。 视频推荐从 **Video Noise Reduction = Native（原厂降噪）**、**Sharpness = Low（低锐度）** 开始。降低降噪会留下更多噪点，增加编码负担。降噪和锐度设得过低时，纹理复杂或运动场景里可能更容易看见压缩伪影。调整后建议按实际使用的码率录一小段再判断。

安装本次更新后，通过手机 App 或 SD 卡安装原厂固件，需要在机身上确认两次。要继续使用 OpenX6，请选「取消」。安装原厂固件将失去 OpenX6 和 root 访问。

**Link Stereo AE / WB** 在拍照和全景录像时都能联动前后镜头的自动曝光与白平衡，已实测 8K50 录像。

**Fisheye 鱼眼预览，显示传感器采集的原生鱼眼画面。** 普通全景录像的左下角视角菜单可选 Native、Linear 或 Fisheye。单镜头 FreeFrame 和全景普通录像分别记住取景选择，重启后也能恢复。开录和停录时，Fisheye 取景随当前档位调整。Flat 和 FreeFrame 下关闭前镜头取景镜像时，现在会保留所选构图，不改变录制文件的方向。选用 360° Audio 时，切回 FreeFrame 或单镜头普通录像会保留该音频模式。FreeFrame 显示当前镜头的画面，普通全景录像可切换前后镜头，分别检查构图。此设置用于机身取景。

### 全景照片、SSD 间隔拍摄与 Fast RAW 后处理

120 MP 拍照可选 **RAW Only**，只保存 DNG。**120 MP Sync** 用于同一台相机的双眼拍摄同步。**RAW Photo Noise Reduction** 提供 Native、Medium、Low、Minimal 四档。

开启 **Use External SSD** 后，可直接录制到兼容的 USB-C exFAT SSD。拍间隔照片时，格式选 **RAW Only**，到 **Mod Settings → Interval Settings** 开启对应分辨率的 **8K Fast RAW** 或 **16K Fast RAW**，间隔选 **Minimum**。相机会根据快门设置和介质实测写速估算目标间隔。**Shutter Max** 可将自动曝光的最长快门设到 2 秒，也可用 **Custom** 手动输入间隔。

**间隔拍摄提速约 20 倍。** 1/100 秒快门、Fast RAW 写入 SSD，连拍 65 张 120 MP DNG 的间隔中位数为 **0.556 秒**。原厂策略对照只拍了两张，起点相隔 **11.296 秒**，包含上一张完成后定时等待的 3 秒。下表是写入 USB-C exFAT SSD、使用手动快门的实拍结果。16K 组使用 1 TB Genki SavePoint。间隔指**相邻两次开始拍摄的时间差**。范围覆盖该组全部相邻间隔。

| Fast RAW 模式 | 快门 | 实存张数 | 间隔中位数 | 最短–最长 |
| --- | ---: | ---: | ---: | ---: |
| 8K / 30 MP，Minimum | 1/100 s | 22 | **0.355 s** | 0.310–0.435 s |
| 16K / 120 MP，Minimum | 1/100 s | 65 | **0.556 s** | 0.543–0.568 s |
| 16K / 120 MP，Minimum | 1/3 s | 62 | **0.874 s** | 0.873–0.876 s |

另一次采用 100 ms 定时器的极限测试中，8K 连拍 65 张的间隔**中位数为 0.33 秒**，最短 **0.29 秒**。16K 在 1/100 秒快门下连拍 82 张，**平均 0.466 秒**，最短 **0.425 秒**。8K 每张 DNG 约 **60 MB**，16K 每张约 **241 MB**。长曝光或 SSD 持续写速下降都会拉长间隔。需要长时间无人值守时，建议在 Minimum 之上留些余量。

**想拍完就得到可调色的 16K RAW？** 选择全景 **Interval（间隔拍照）→ 16K（120 MP）→ 1:1 → RAW Only**。开始这一组拍摄前，进入 **Mod Settings → Interval Settings**，关闭 **16K Fast RAW**，把 **Interval Timing** 设为 **Minimum**，再按快门开始。相机会边拍边完成 PureShot 处理，写出的就是可直接调色的标准 Bayer DNG。

短快门下可按**约 4.5–5 秒一张**估算。内置存储、手动 **1/100 秒**时，实拍 4 张，相邻快门启动间隔分别为 **5.075、4.516、4.550 秒**，中位数 **4.55 秒**（约 **0.22 张/秒**）。另一次 SSD 实拍使用自动曝光、快门上限 1/8 秒，3 张成片的两次写盘时间差为 **4.46 和 4.56 秒**。长曝光或较慢的写盘会拉长间隔。此模式的 **Custom** 最低可设 **4.0 秒**，实际仍要等上一张处理完成才能拍下一张。

**为什么 16K Fast RAW 拍完还要处理？** 为了缩短拍摄间隔，相机会先把 120 MP 传感器的 **Quad Bayer** 数据直接存进 DNG。它的每个 2 × 2 像素块都采集同一种颜色。常规 RAW 软件按普通 Bayer 排列解读时，即使能打开文件，也可能看到棋盘格或错误的颜色。拍完后，机内 PureShot 会把这些采样转换成标准 Bayer 排列，保留 **15520 × 7760** 的完整分辨率，得到可按常规流程调色的 DNG。8K Fast RAW（7776 × 3888）可直接使用，无需这一步。两个模式都可以在上一张写盘时开始拍下一张。

**开启 16K Fast RAW 拍完后**，保持 SSD 连接，进入 **Mod Settings → 16K Fast RAW Processing**，选中拍摄组，点 **Start processing**。相机会逐张校验，然后原地替换 DNG。想保留 Quad Bayer 原片，请在开始处理前先拷贝。一组 34 张实拍的后处理平均耗时 **5.7 秒/张**。

**16K 延时视频**直接生成视频，可单独开启**每眼 245 Mbps**。上表的亚秒级间隔仅适用于间隔 DNG 照片。

### 存储、音频与机身设置

Camera Info 显示 `1.2.36 OpenX6 1.1`。单镜头视频支持 iLog 和空间音频。Wi-Fi 空闲超时可选常开、2、5 或 10 分钟。SD 更新界面显示校验与复制进度，兼容版本重装保留原有设置和 Holder 密钥。统一更新包含 MCU 1.2.0 应用，已是该版本时自动跳过写入。

### 先看机身当前版本

| 核对项目 | 本版支持范围 |
| --- | --- |
| 桌面构建输入 | 官方 X6 **1.2.36** 文件。原厂 **0.8.825**（实机验证）或 **0.8.827**（离线验证）可通过热点解包，后续构建可复用电脑上已校验的模块。 |
| 原厂机首次安装 | 支持出厂 **0.8.825** 或 **0.8.827**，同时核对原厂升级器和分区布局。请保留机身的早期固件，直接安装 OpenX6。已经运行原厂 **1.2.36** 或 **1.3.59** 的机器无法安装此卡刷包。 |
| 已改装机更新 | 支持已运行兼容改装版本、且具备 SD 维护更新入口的 X6。请备份首次构建生成的 `Holder` 目录，后续构建继续使用。 |

在可写 SD 卡根目录放入 `OPENX6-BOOT.TXT` 可以请求一次排障启动。写入 `LEVEL1` 跳过 Mod，`LEVEL2` 进入救援，或 `NORMAL` 取消待执行请求。二级保护下，热点的 23456 端口仍提供 root 命令服务。相机会通过维护 SSH 或热点服务在机内验证 root，两者任一通过且升级器空闲，即可把新槽标记成功；两者都失败时暂缓标记。

### 开始使用

桌面应用首次启动为英文。从 **Language → 简体中文** 切换语言，下次启动会沿用选择。

Apple 芯片 Mac 打开 **OpenX6.app**；Windows 11 x64 完整解压原生 ZIP 后打开 **OpenX6.exe**。选择官方 1.2.36，把电脑连到原厂 0.8.825 或 0.8.827 相机热点。点「在我的 X6 上解包」。核对序列号后六位，若机身弹窗则点「允许」。保持相机开机，等三个模块校验完成即可断开热点，在电脑上构建。0.8.827 机内解包待实机验证。

选好官方固件和三个模块后开始构建。**不要把 Insta360 官方固件包拷入 SD 卡。** OpenX6 校验通过后，只将生成的 `OpenX6 SD Build/Insta360X6FW.bin` 拷到 SD 卡根目录。安全弹出卡，插入相机，保持供电直至升级完成。

连接相机 Wi-Fi 热点的设备可以执行 root 命令，请妥善保管热点密码。

**验证。** 本次 Windows ZIP 已在 Windows 11 x64 上通过原生启动、语言切换和沿用原 Holder 的完整构建，成品也通过了独立验签。该成品的新机首装尚待验证。

### 模块框架：开发预览

**现在能试什么。** 有 Python 和仓库源码，就可以在电脑上预演录像容量估算示例的菜单，选择码率，再运行处理程序，估算每小时录像占用多少空间。要试写自己的模块，运行 `python3 -m sdk init ../field_budget` 创建示例，再用 `python3 -m sdk check ../field_budget/module.json` 校验。在开发者工具中，可以用官方 1.2.36 和选定的模块构建候选包，再从成品读回并核对菜单、程序和启动服务。这类候选包带安装锁，仅供离线检查；目前可安装的卡刷包只包含内置功能。[SDK 入门](sdk/README.zh.md#用-python-创建第一个模块)

**后续计划。** 我们希望作者可以独立维护自己功能的菜单、代码和测试，让用户在构建可安装的卡刷包时，能把兼容的社区模块与内置功能一起选入，由核心负责兼容性检查和成品校验。模块清单格式和菜单接口仍可能调整。

OpenX6 核心采用 **GPL-3.0-only**，模块 SDK 和随附示例采用 **Apache-2.0**。

详细操作见[中文使用说明](README.zh.md)。

Thanks to derrickyau9 and the contributors who documented the Luna Ultra. 感谢 derrickyau9 和贡献者公开相关研究。[Upstream research / 上游研究](https://github.com/derrickyau9/insta360-luna-ultra-reverse-engineering)

**Mac automatic update fix.** The app now confirms a successful update when macOS opens it from an App Translocation path. Reopening the original app then launches the updated version. Existing Holder identity, module cache and language preference are preserved.

**Mac 自动更新修正。** 修复 macOS 从隔离路径打开新版后未完成更新收尾的问题。重新打开原入口会进入已更新的版本，已有 Holder 身份、模块缓存和语言偏好保持不变。
