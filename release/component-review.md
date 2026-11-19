# Component inventory

The signed 1.2.36 profile covers 931 inputs. The builder verifies 876 files from the checkout, including vendored dependencies and two synthetic encoder statistics fixtures. It derives the other 55 from the official system image. Every input has an expected SHA-256.

### 16K Fast RAW

The local recipe verifies the ISP library and extracts 41 floats from the dual-eye 120 MP snapshot descriptor for chromatic aberration, sharpening and noise add-back. On conversion each DNG supplies ISO, exposure, white balance, its color matrix and four lens gain maps. The writer preserves the original color tags and gain maps. The engine uses an identity working matrix and leaves its optional internal shading table empty.

On a reference shot, archived native-engine tests of the working matrix and shading table individually found zero changed pixels per eye. Desktop replay using the recorded inference outputs reproduced three complete reference DNGs byte for byte. This checks both eye inputs, image assembly and rendering metadata. Combined native inference across dark and high-gain scenes remains unverified. The converter selects the baseline lux band because the supported DNGs lack a verified native lux index. Keep this converter in the locked preview pending that comparison.

### Checking the media service

The [media recipe](../openx6/mediad_recipe.py) lists the SHA-256 of each intermediate executable. It compiles the [C and AArch64 hooks](../recipes/mediad/) in order, including older hooks whose bytes remain in the final file. The builder rejects the first mismatching layer. With no third-party media module selected, the expected SHA-256 of `/usr/bin/ins_mediad` inside the candidate is `dbc3ef7b694bd45719a3eb750586b762f794f55aa8d379a4786bba56c8848ea5`. A selected media module applies after this check and records its own output digest in `release-inputs.json`.

### Checking the camera HAL

The [HAL recipe](../openx6/camera_hal_recipe.py) compiles the [OpenX6 exposure, HNR and interval hooks](../recipes/camera_hal/). The installed HAL at `/usr/lib/hw/camera.qcom.so` has SHA-256 `8386b304b83aab877c469703bae50fd8150ac26a9cf5d730d4c90725bb673a97`. The build derives both legacy HNR variants from that installed file, checks each intermediate digest, then embeds their interval-aware deltas in `x6-mod-controls-prepare`. Video HNR defaults to the native setting.

The [interval auto-exposure recipe](../openx6/interval_ae_recipe.py) compiles the still-capture shutter limit into `com.insta.stats.aec.so`. The [SSD recipe](../openx6/ssd_recipe.py) builds the storage-selection hooks for `ins_configd` and `ins_bla` from the [same branch layout](../recipes/ssd/layout.json) used for the SSD coordinator metadata. Each finished executable must match its signed digest.

The [ADB recipe](../openx6/adb_recipe.py) rebuilds the daemon, authentication library and systemd unit. The daemon requires native ADB authentication, and the library uses the holder key at `/adb_keys` with interactive enrollment disabled.

### Checking the maintenance updater

The [updater recipe](../openx6/otad_recipe.py) builds the [plaintext entry and version hooks](../recipes/otad/gate.S). The updater sends the exact privately staged path to `x6-owner-update-check`; encrypted packages continue through the vendor entry. For an accepted main-system package the native version comparison permits an equal version. An older version needs a per-package downgrade result from the verifier. The SD policy also accepts structurally valid unsigned plaintext and derives its downgrade decision from the package and base versions. A malformed signature on a recognized signed record is rejected. The locked desktop preview keeps installation disabled.

### Checking factory USB and ADB

The [USB recipes](../openx6/factory_usb_recipe.py) rebuild the hardware service, USB receiver and factory API from the official image. The hardware service selects an ADB composition and checks the stop helper before switching. ADB requires an RSA challenge response with the holder key. Factory requests use a separate Ed25519 challenge. The receiver checks the payload length before copying; the [factory gate](../recipes/factory_usb/factory_gate.S) consults the authentication service before dispatching each request. A valid request reaches the native parser with its 15-byte inner header.

[Stock generation rules](../components/stock-rules.json) · [Notices](../NOTICE.md)
