# 模块开发说明

[English](README.md)

**开发预览。** 本指南介绍当前可试用的模块接口。清单和菜单接口可能调整。查看[框架现状与后续方向](../README.zh.md#模块框架开发预览)。

## 用 Python 创建第一个模块

在仓库根目录用 Python 3.11 或更高版本运行下面三条命令。到这一步只需要源码和 Python。Windows 上可将 `python3` 换成 `py -3`。

```sh
python3 -B -m sdk init ../field_budget
python3 -B -m sdk check ../field_budget/module.json
python3 -B -m sdk.menu_preview ../field_budget/module.json --set field_budget.rate max
```

第一条创建新目录，里面有定义相机菜单的 `module.json`、估算录像容量的 `budget.py` 和模板的 Apache-2.0 许可证 `LICENSE`。预演选 Max 应显示 **260 Mbps**。要改成 240，分别修改清单里的选项文字和程序中 `max` 对应的码率。再运行 `check`，它会指出源码摘要已变化，并给出当前文件的 SHA-256。核对改动后，将该值填入 `program.sha256`，再检查一次。要运行自己写的计算，可执行 `python3 -I -B ../field_budget/budget.py max`，此时应返回每小时 **100.58 GiB**。

一组模块可以一起传给 `check`。预检校验清单、所选模块间的冲突、程序摘要及 Python 3.10 语法，全程不执行模块代码。默认使用完整构建档位；检查原厂组件字节补丁时加 `--profile stock`。开发者桌面工具选对应档位、添加相同的清单，点 **预检模块** 也能得到结果。正式核对原字节、内置补丁的覆盖范围和菜单容量，需要再用官方 1.2.36 构建锁定候选包。

交给其他开发者时，一起提供清单、同目录的程序源码、许可证和一组输入及预期输出。贡献到本仓库的新插件采用 Apache-2.0。第三方独立发布的插件沿用其代码和依赖各自适用的许可。接收方可复用以上步骤独立预检和构建；当前普通用户的卡刷构建仅包含内置模块。

## 编写字节补丁

在 SDK 目录运行 `python3 -B module_plan.py --base examples/sample.txt --target example examples/demo.json`，可对合成文本作只读预检。

复制一个示例 JSON，给模块起独立 ID，填写目标组件的精确 SHA-256、依赖、冲突及修改项。每项由字节偏移、原字节和等长替换字节组成，使用小写十六进制。所需依赖须同时选中。修改重叠、原字节不符和错误基础摘要均会被拒绝。

默认档位选 [stock_debug_label.json](examples/stock_debug_label.json)，摘要针对官方 1.2.36 媒体程序。完整构建档位选 [media_debug_label.json](examples/media_debug_label.json)，摘要针对已应用内置补丁的媒体程序。两者都将 ELF 调试符号文件名改为 `ox6_mediad`。构建后可从 `release-inputs.json` 核对清单和组件摘要。

贡献内容应包含原创模块清单、合成测试和核对修改的步骤。固件、密钥和本地生成物保留在仓库外。[桌面操作流程](../README.zh.md)

## 试写相机菜单（电脑预演）

复制 [menu_grid.json](examples/menu_grid.json)，为模块起一个独立 ID。每一行用 `toggle` 或 `choice` 声明，默认值须与类型匹配。在本目录运行：

```sh
python3 -B menu_preview.py examples/menu_grid.json --set grid_guide.enabled on --set grid_guide.style Center
```

输出包含页面、行标题、可选项和这次选择的值。参考线示例使用仅预演的清单格式，选择保存在本次进程内。上面的 `field_budget` 示例使用可构建的原生菜单格式。

## 构建相机原生菜单项

在开发者桌面工具勾选「完整 OpenX6 构建」，选择官方 1.2.36、自己的机内导出模块目录和持有者配置，添加 [stats_overlay.json](examples/stats_overlay.json)，生成锁定候选包。相机菜单路径为 **Mod Settings → Extensions → Capture statistics → Show recording stats**。原 Stats 开关移到这里，沿用已经保存的偏好。省略模块即可构建标准菜单布局。

复制示例即可修改模块 ID、页面标题和开关文字；原生文字目前使用可打印 ASCII。第一个可用绑定 `stats.overlay.enabled` 接受布尔值，全新配置默认关闭。已有设置由原统计服务读取。两个模块占用同一绑定会在解密前被拒绝。新增其他绑定需要给核心提供对应的行为和测试。上方的参考线示例仍供桌面预演。

可在仓库根目录运行 `python3 -B sdk/menu_preview.py sdk/examples/stats_overlay.json --set capture_stats.enabled on` 预演。构建后打开 `menu-build.json` 核对生成的 GUI 摘要，打开 `RESULT.json` 核对 `install_allowed: false`。

在仓库根目录用同一套工具执行以下命令，可以从成品内重建菜单并逐字节核对：

```sh
python3 -B -m sdk.verify_menu_candidate "/path/to/OpenX6 Build" sdk/examples/stats_overlay.json
```

## 让模块拥有自己的单选设置

复制 [field_checklist.json](examples/field_checklist.json)，修改模块 ID、页面标题和选项。这个示例在 **Mod Settings → Extensions → Field checklist** 显示三种机位。选 Vehicle 时，页面提示核对固定座与安全绳。每份清单定义一个设置，支持 2–8 个原生单选项，每项的 `note` 由贡献者填写。多个这样的模块可以一起构建。

先在电脑预演，再在开发者桌面工具的「完整 OpenX6 构建」中添加清单：
```sh
python3 -B sdk/menu_preview.py sdk/examples/field_checklist.json --set field_checklist.rig vehicle
```
模块、设置和选项的 `id` 是持久化接口。升级时可改显示文字、调整顺序；保留 ID 就会沿用已有选择。若移除用户已选的选项，菜单会请用户重新选择，在此之前保留原值。保存失败会显示提示并重新读取文件。

模块侧可使用 Python 的 `sdk.choice.read_choice(directory, doc)` 读取选项 ID。传入偏好所在目录和已解析的清单；本地测试可用临时目录，相机上的目录是 `/data/x6-owner-features/extensions-v1`。该设置的文件为 `openx6-choice.<模块ID>.<设置ID>.pref`，内容是所选 ID 的 ASCII 字节，无换行。初次读取返回声明的默认值。

若同时构建 Stats 示例，读回时把两份清单一起传给 `sdk.verify_menu_candidate`。

可以用独立的小客户端读取本地偏好目录的副本：
```sh
python3 -B -m sdk.examples.read_checklist "/本地偏好目录"
```
空目录显示 Tripod 的默认提示。目录有保存值时，客户端读取选项 ID，再按模块自己的映射显示对应提示。

## 用菜单驱动自己的程序

[存储预算示例](examples/recording_budget/module.json)把单选菜单和同目录的 [budget.py](examples/recording_budget/budget.py) 一起打包。选择 **Mod Settings → Extensions → Storage estimate → 260 Mbps**，模块会按视频载荷算出每小时 108.96 GiB。它只做容量估算；实际录像码率由拍摄菜单控制。

1. 复制整个示例目录，把 [SDK 许可证](LICENSE) 一起保存为目录下的 `LICENSE`。选择一个稳定的模块 ID、设置 ID 和选项 ID。模块程序使用相机已提供的 Python 3.10 标准库，首个参数是选项 ID。在标准输出写一个 UTF-8 JSON 对象（最多 8192 字节），成功时退出码为 0。可先执行 `python3 -I -B budget.py max` 检查自己的代码。
2. 修改源码后，执行 `shasum -a 256 budget.py`（或 `sha256sum budget.py`），把摘要填入清单的 `program.sha256`。`source` 是同目录的单个 Python 文件名。构建阶段读取和校验源码，不运行模块，也不安装 pip 依赖。
3. 开发者桌面工具选「完整 OpenX6 构建」，像上节一样选择官方包、材料和持有者配置。点「添加」，选示例的 `module.json`，再生成锁定候选包。多个模块可共用一次构建。
4. 从仓库根目录执行下面的回读命令。若选了其他菜单模块，也一并列出。工具会核对成品内的菜单、模块源码、启动服务及其启动链接。回读命令本身不会执行第三方代码。

```sh
python3 -B -m sdk.verify_menu_candidate "/本地/OpenX6 Build" sdk/examples/recording_budget/module.json
```

核心从受保护的持久化偏好中取出已选 ID，交给该模块的独立低权限工作进程。工作进程启动和选项改变时重新计算，选择未变时复用成功结果。它面向可重复执行的计算，快速切换时以最新保存值为准。每次计算限时 3 秒；失败后移除该次输出并重试。示例结果保存在 `/run/openx6-result-recording_budget/result.json`，格式为 `{"selection":"max","result":{"mbps":260,"video_gib_per_hour":108.96,"minutes_per_128_gib":70.5}}`。正常拍摄无需访问这个文件；开发者可从持有者环境读取。安装守卫继续锁定。

构建器按原厂策略给模块服务和启动链接标记类型。GUI 启动前，只有 root 可执行的准备程序会创建私有偏好目录，迁移所选菜单声明的偏好文件及其原子保存临时文件，并保留原值。遇到符号链接、具有多个硬链接的偏好文件、异常属主或意外标签时，准备流程会拒绝继续。模块以独立的动态 UID 读取选项 ID，并把结果写入自己的私有运行目录。

离线 ARM64 虚拟机使用原厂 1.2.36 的 systemd 和 MLS 策略。在 permissive 和 enforcing 两种模式下，服务自启、菜单原生回调保存、模块执行和重启后恢复选项均通过。模块直接读取持有者偏好或其他模块的私有结果目录均被拒绝。虚拟机使用通用内核并提供启动前置条件；真机启动顺序、叠加挂载后的标签和触屏操作仍需实测。

原厂 systemd 编译时关闭了 seccomp，模块仍可在自己的隔离网络命名空间内创建套接字。构建前请审查第三方代码，源码摘要用于固定已审查的版本。
