# Windows 11 x64

For the Windows 11 x64 consumer app, see the [native launcher, GUI and packaging instructions](native/README.md).

## WSL developer environment

The WSL package includes a native file picker, a dedicated x86-64 WSL 2 distribution, and a Windows-native camera module exporter. See [the bilingual Windows instructions](START-HERE.txt) for the user workflow. The Linux worker must pass a real offline 1.2.36 build before the packager exports a clean runtime. Windows-host acceptance is a separate step.

## Publisher build

Supply a clean local Linux/amd64 OCI image with Python 3.11+, OpenSSL 3, OpenSSH, e2fsprogs, useradd/runuser, Go 1.27.3 in /opt/toolchain/go, Clang and llvm-strip 22.1.4, GNU-flavor LLD and Zig 0.17.0 in /opt/toolchain/bin, and Zig's library in /opt/toolchain/zig-lib. Preserve toolchain and OS license notices. Go on the publisher's host builds the Windows launcher.

Run `python3 package.py --toolchain-image <local sha256 image ID> --firmware <official file> --modules <exported directory> --output <new directory>` from this directory. The disposable build container reads the mounted module folder and publishes a verified package to an ephemeral Windows-style path. Only a fresh, unused container is exported. The launcher pins that runtime tar by SHA-256.

Run `python3 add_reader.py <release directory> <python-3.14.9-embed-amd64.zip>` with the official Python.org archive. The packager checks its published SHA-256. Include START-HERE.txt in the release. The native reader connects only after the owner explicitly starts it. Its transport is adapted from the Apache-licensed X6 research client. The offline builder can run independently.

On Windows, test fresh WSL import, paths with spaces and non-ASCII characters, rejection of an incomplete module folder and unsupported firmware, existing output protection, and a successful build with output SHA-256 readback. Reuse the same Windows user and WSL distribution to retain the holder identity.

## Updating the consumer program

Run `python3 refresh_release.py <qualified-release.zip> <new-release.zip>` for an application update on the current WSL base. Use the release ZIP matching the SHA-256 pinned in the script. It verifies the prior release and runtime, bundles tracked OpenX6 build sources, and pins the new program archive in the new EXE. Extract the entire release under the same Windows account. The launcher checks the registered base, reads the bundled program into a private temporary directory, and runs under the original `openx6` account. Each successful consumer build uses that account’s Holder. Both initial base markers are accepted for this release. A different Linux toolchain requires its own qualification.

A first import may enroll a Holder. Once enrolled, a missing Holder or a change to its pinned public profile stops the build. A known older worker with no Holder needs its original backup to qualify as an upgrade. The exclusive guest lock serializes current launchers. Quit the legacy launcher before opening the update. The two-binary Windows acceptance walkthrough is in [consumer upgrade acceptance](acceptance/consumer/WINDOWS-TEST.md).
