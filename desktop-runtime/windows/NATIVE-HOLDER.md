# Windows native Holder

Extract the full ZIP and run OpenX6.exe using the same Windows account for subsequent builds. The private keys are protected by Windows CurrentUser DPAPI. Back up the Windows user profile, including its DPAPI data, the OpenX6 Native Holder folder and its registry identity record. Copying the Holder folder alone does not transfer signing access to a new Windows account. Preserve the original environment when switching machines. An older WSL Holder requires its original environment.

请在同一 Windows 账户下运行后续版本。私钥由当前用户的 DPAPI 保护。备份应包含 Windows 用户配置文件及其 DPAPI 数据、OpenX6 Native Holder 目录和注册表中的身份记录。仅复制 Holder 文件夹无法在新账户恢复签名能力。更换机器前请保留原环境。
