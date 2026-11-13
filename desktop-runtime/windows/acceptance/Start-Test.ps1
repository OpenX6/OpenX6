param([switch]$Gui)
$ErrorActionPreference = 'Stop'
$distro = 'OpenX6-DevQA'
$runtime = Join-Path $PSScriptRoot 'OpenX6-runtime.tar'
$marker = 'd8c99be83e1edcd47cd636e1356bf29313f079c35b5a40c9760e89ff302e399e'
$null = Get-Command wsl.exe -ErrorAction Stop
Add-Type -AssemblyName System.Windows.Forms
function WslPath([string]$path) {
    if ($path -notmatch '^[A-Za-z]:\\') { throw 'Choose a path on a local Windows drive.' }
    $drive = $path.Substring(0,1).ToLowerInvariant()
    return '/mnt/' + $drive + $path.Substring(2).Replace('\','/')
}
function PickFile([System.Windows.Forms.Form]$ownerWindow, [string]$title) {
    $picker = New-Object System.Windows.Forms.OpenFileDialog
    try {
        $picker.Title = $title
        if ($picker.ShowDialog($ownerWindow) -eq [System.Windows.Forms.DialogResult]::OK) {
            return $picker.FileName
        }
    } finally { $picker.Dispose() }
}
$ownerWindow = New-Object System.Windows.Forms.Form
try {
    $ownerWindow.Text = 'OpenX6 Developer Acceptance'
    $ownerWindow.StartPosition = 'CenterScreen'
    $ownerWindow.FormBorderStyle = 'FixedDialog'
    $ownerWindow.MaximizeBox = $false
    $ownerWindow.ClientSize = [System.Drawing.Size]::new(440, 72)
    $prompt = New-Object System.Windows.Forms.Label
    $prompt.Dock = 'Fill'
    $prompt.TextAlign = 'MiddleCenter'
    $prompt.Text = 'Choose the official X6 firmware, then its exported three-module folder.'
    $ownerWindow.Controls.Add($prompt)
    $ownerWindow.Show()
    $ownerWindow.Activate()
    $firmware = PickFile $ownerWindow 'Select official X6 1.2.36 firmware'
    if (-not $firmware) { return }
    $folder = New-Object System.Windows.Forms.FolderBrowserDialog
    try { if ($folder.ShowDialog($ownerWindow) -ne 'OK') { return }; $material = $folder.SelectedPath } finally { $folder.Dispose() }
    if (-not $material) { return }
} finally {
    $ownerWindow.Dispose()
}
$names = @(& wsl.exe --list --quiet | ForEach-Object { ($_ -replace '\x00','').Trim() })
if ($names -notcontains $distro) {
    Write-Host 'Checking bundled WSL runtime...'
    if ((Get-FileHash -LiteralPath $runtime -Algorithm SHA256).Hash -ne 'c52aa2e6d15820457b655cc8df5341d40f1cdcd28aa398b63928d6af9365c169') {
        throw 'Runtime SHA-256 mismatch.'
    }
    $location = Join-Path $env:LOCALAPPDATA 'OpenX6\DevQA'
    New-Item -ItemType Directory -Force -Path $location | Out-Null
    & wsl.exe --import $distro $location $runtime --version 2
    if ($LASTEXITCODE -ne 0) { throw 'WSL import failed. Check wsl --status and available disk space.' }
}
$installed = (& wsl.exe -d $distro -u openx6 -- /bin/cat /opt/openx6-worker-version) -join ''
if ($LASTEXITCODE -ne 0 -or $installed.Trim() -ne $marker) { throw 'Unexpected OpenX6-DevQA runtime.' }
$kitPath = WslPath $PSScriptRoot
$fwPath = WslPath $firmware
$materialPath = WslPath $material
$mode = if ($Gui) { 'gui' } else { 'check' }
$previousEncoding = [Console]::OutputEncoding
try {
    # Windows PowerShell 5.1 uses this encoding to decode native stdout.
    [Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
    $returnedPath = & wsl.exe -d $distro -u openx6 --exec /usr/bin/printf '%s\n' $fwPath
    if ($LASTEXITCODE -ne 0 -or -not [string]::Equals($fwPath, $returnedPath, [StringComparison]::Ordinal)) {
        throw 'WSL firmware path did not round-trip as UTF-8.'
    }
    Write-Output "WSL UTF-8 path OK: $returnedPath"
    & wsl.exe -d $distro -u openx6 --exec /bin/bash "$kitPath/run-check.sh" "$fwPath" "$materialPath" $mode | Write-Output
    if ($LASTEXITCODE -ne 0) { throw 'Developer acceptance stopped. See the preceding message.' }
} finally {
    [Console]::OutputEncoding = $previousEncoding
}
