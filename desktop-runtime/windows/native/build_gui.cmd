@echo off
setlocal
if "%~1"=="" (
  echo Usage: build_gui.cmd path-to-staging-directory
  exit /b 2
)
if not exist "%~1\." exit /b 2
"%WINDIR%\Microsoft.NET\Framework64\v4.0.30319\csc.exe" /nologo /optimize+ /codepage:65001 /target:winexe /platform:anycpu /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /out:"%~1\OpenX6.Desktop.exe" "%~dp0GUI\BuilderForm.cs" "%~dp0GUI\OpenX6Job.cs" "%~dp0GUI\DesktopLanguage.cs"
exit /b %errorlevel%
