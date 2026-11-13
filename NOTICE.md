# Notices

## OpenX6 licensing

The OpenX6 contributors license their original work in this repository under the GNU General Public License, version 3 only (`GPL-3.0-only`), except where a more specific license is identified. This covers the core engine, desktop builders, camera maintenance tools and built-in firmware patch recipes. The reusable module SDK and bundled example plugins under `sdk/` use the Apache License, Version 2.0 (`Apache-2.0`), including the template copied by `python -m sdk init`. Contributions to the core and SDK follow their respective licenses. New plugins contributed to this repository use Apache-2.0. [Core license](LICENSE) · [SDK and plugin license](sdk/LICENSE)

The SDK command-line tool uses the GPL-licensed build engine. When publishing OpenX6 binaries, provide the corresponding source for that revision, including its build and packaging scripts. Third-party files retain their existing copyright notices and licenses. Firmware and other components supplied in the user's official update remain subject to their original licenses and notices, including those for the Linux kernel.

## Third-party notices

OpenX6 is an independent project. Insta360 firmware and device credentials are supplied locally by the user. The standalone macOS app bundles Python/Tk, compilers, OpenSSL and ext4 utilities. Its runtime notices are in Contents/Resources/tools/notices. The developer launcher uses locally installed tools.

The OpenX6 source tree includes golang.org/x/crypto v0.58.0 and golang.org/x/sys v0.49.0. Their original LICENSE and PATENTS files accompany their sources under components/src/vendor/golang.org/x/{crypto,sys}.

The Windows acceptance bundle includes an x86-64 Debian toolchain. OS notices are under /usr/share/doc, compiler and language notices under /opt/toolchain. Windows CPython ships its license in Reader/Python. The shared camera_reader module contains the Apache-2.0 notice and license for the optional Wi-Fi transport. The Mac app includes them in Contents/Resources/camera_reader.
