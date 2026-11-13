$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[Windows.Forms.Application]::EnableVisualStyles()

# Drain both redirected streams outside the UI thread, including large compiler output.
Add-Type -TypeDefinition @'
using System;
using System.Collections.Concurrent;
using System.Collections.Generic;
using System.Diagnostics;
using System.Text;

public sealed class OpenX6Job : IDisposable {
    readonly Process process = new Process();
    readonly ConcurrentQueue<string> lines = new ConcurrentQueue<string>();
    public string LastOutput { get; private set; }
    public string LastError { get; private set; }

    public OpenX6Job(string exe, string action, Dictionary<string,string> environment) {
        var info = new ProcessStartInfo(exe, action) {
            UseShellExecute = false, CreateNoWindow = true,
            RedirectStandardOutput = true, RedirectStandardError = true, RedirectStandardInput = true,
            StandardOutputEncoding = Encoding.UTF8, StandardErrorEncoding = Encoding.UTF8
        };
        foreach (var pair in environment) info.EnvironmentVariables[pair.Key] = pair.Value;
        process.StartInfo = info;
        process.OutputDataReceived += (s,e) => {
            if (!String.IsNullOrWhiteSpace(e.Data)) { LastOutput = e.Data; lines.Enqueue(e.Data); }
        };
        process.ErrorDataReceived += (s,e) => {
            if (!String.IsNullOrWhiteSpace(e.Data)) { if (LastError == null || (!e.Data.StartsWith("exit status ") && !e.Data.StartsWith("build stopped;"))) LastError = e.Data; lines.Enqueue(e.Data); }
        };
        process.Start();
        process.BeginOutputReadLine();
        process.BeginErrorReadLine();
    }

    public bool Finished() {
        if (!process.HasExited) return false;
        process.WaitForExit(); // Flush both async readers before interpreting the signed build footer.
        return true;
    }
    public int ExitCode { get { return process.ExitCode; } }
    public string NextLine() { string value; return lines.TryDequeue(out value) ? value : null; }
    public void CancelExport() {
        try { process.StandardInput.BaseStream.WriteByte(0x43); process.StandardInput.BaseStream.Flush(); }
        catch (System.IO.IOException) { }
    }
    public void Dispose() { process.Dispose(); }
}
'@

$form = New-Object Windows.Forms.Form
$form.Text = 'OpenX6 · SD Card Builder'
$form.StartPosition = 'CenterScreen'
$form.AutoScaleMode = 'Font'
$form.Font = [Drawing.Font]::new('Segoe UI', 10)
$form.ClientSize = [Drawing.Size]::new(786, 354)
$form.MinimumSize = [Drawing.Size]::new(730, 385)
$menu = New-Object Windows.Forms.MenuStrip
$file = $menu.Items.Add('File')
$selectModules = $file.DropDownItems.Add('Choose saved module folder… / 选择已保存的模块目录…')
$updateItem = $file.DropDownItems.Add('Check for updates… / 检查更新…')
$form.MainMenuStrip = $menu
$grid = New-Object Windows.Forms.TableLayoutPanel
$grid.Dock = 'Fill'
$grid.Padding = [Windows.Forms.Padding]::new(16)
$grid.ColumnCount = 3
foreach ($style in @('AutoSize','Percent','AutoSize')) {
    $col = [Windows.Forms.ColumnStyle]::new()
    $col.SizeType = $style
    if ($style -eq 'Percent') { $col.Width = 100 }
    [void]$grid.ColumnStyles.Add($col)
}
function Add-Control($control, $column, $row, $span=1) {
    $control.Anchor = 'Left, Right'
    $control.Margin = [Windows.Forms.Padding]::new(4)
    $grid.Controls.Add($control, $column, $row)
    $grid.SetColumnSpan($control, $span)
}
function New-Label($text) {
    $label = New-Object Windows.Forms.Label
    $label.AutoSize = $true
    $label.Text = $text
    return $label
}
function New-Button($text) {
    $button = New-Object Windows.Forms.Button
    $button.Text = $text
    $button.AutoSize = $true
    return $button
}
function New-Field {
    $field = New-Object Windows.Forms.TextBox
    $field.ReadOnly = $true
    return $field
}
Add-Control (New-Label "For Wi-Fi deployment, insert an SD card with no update files.`n如需通过 Wi-Fi 部署，请先在相机内插入一张没有升级包的 SD 卡。") 0 0 3
Add-Control (New-Label 'Official X6 firmware / 官方固件') 0 1
$fw = New-Field
Add-Control $fw 1 1
$choose = New-Button 'Choose / 选择'
Add-Control $choose 2 1
Add-Control (New-Label 'Build input / 构建材料') 0 2
$modules = New-Field
Add-Control $modules 1 2
$read = New-Button 'Unpack on my X6 / 在我的 X6 上解包'
Add-Control $read 2 2
$info = New-Label ''
Add-Control $info 0 3 3
$cancel = New-Button 'Cancel export / 取消导出'
Add-Control $cancel 0 4
$build = New-Button 'Build SD package / 生成卡刷包'
Add-Control $build 2 4
$status = New-Label 'X6 · 1.2.36'
$status.MaximumSize = [Drawing.Size]::new(725, 72)
Add-Control $status 0 5 3
$bar = New-Object Windows.Forms.ProgressBar
Add-Control $bar 0 6 3
$actions = New-Object Windows.Forms.FlowLayoutPanel
$actions.AutoSize = $true
$actions.WrapContents = $true
$folder = New-Button 'Open output folder / 打开固件所在目录'
$deploy = New-Button 'Deploy to SD via Wi-Fi / 通过 Wi-Fi 部署到 SD 卡'
$actions.Controls.AddRange(@($folder,$deploy))
Add-Control $actions 0 7 3
$form.Controls.Add($grid)
$form.Controls.Add($menu)

$cache = Join-Path $env:LOCALAPPDATA 'OpenX6\Camera\X6-1.2.36-modules'
$state = @{job=$null; kind=''; checked=$false; output=''; digest=''; check=$null; update=''}
function Clear-Build {
    $state.output = ''
    $state.digest = ''
    $actions.Visible = $false
}
function Refresh-Controls {
    $idle = $null -eq $state.job
    $cached = $modules.Text -eq $cache -and [IO.Directory]::Exists($cache)
    $choose.Enabled = $idle
    $selectModules.Enabled = $idle
    $read.Enabled = $idle -and ($cached -or $state.checked)
    $read.Text = if ($cached) { 'Open cache folder / 打开缓存目录' } else { 'Unpack on my X6 / 在我的 X6 上解包' }
    $info.Text = if ($cached) { 'Saved on this PC · ready for offline builds / 已找到本机缓存，下次会自动使用' }
        elseif ($modules.Text) { 'Selected module folder / 已选择模块目录' }
        else { 'First use: connect to X6 Wi-Fi to unpack / 首次使用请连接相机热点解包' }
    $build.Enabled = $idle -and $state.checked -and $modules.Text.Length -gt 0
    $actions.Visible = $state.digest.Length -gt 0
    $actions.Enabled = $idle
    $cancel.Visible = $state.kind -eq 'export'
    $bar.Visible = !$idle
    $updateItem.Enabled = $idle -and $null -eq $state.check
}
function New-JobProcess($action) {
    $values = [Collections.Generic.Dictionary[string,string]]::new()
    $values.Add('OPENX6_FW',$fw.Text)
    $values.Add('OPENX6_MODULES',$modules.Text)
    $values.Add('OPENX6_OUTPUT',$state.output)
    $values.Add('OPENX6_DIGEST',$state.digest)
    $values.Add('OPENX6_UPDATE',$state.update)
    return [OpenX6Job]::new($env:OPENX6_SELF, $action, $values)
}
function Start-Action($kind, $message) {
    if ($state.job) { return }
    try {
        $state.job = New-JobProcess "--ui-$kind"
        $state.kind = $kind
        $status.Text = $message
        $bar.Style = 'Marquee'
        $cancel.Enabled = $true
    } catch { $status.Text = $_.Exception.Message }
    Refresh-Controls
}
function Select-Folder($description) {
    $dialog = New-Object Windows.Forms.FolderBrowserDialog
    try {
        $dialog.Description = $description
        if ($dialog.ShowDialog($form) -eq 'OK') { return $dialog.SelectedPath }
    } finally { $dialog.Dispose() }
}
$choose.Add_Click({
    $dialog = New-Object Windows.Forms.OpenFileDialog
    try {
        $dialog.Title = 'Select official X6 1.2.36 firmware / 选择官方固件'
        if ($dialog.ShowDialog($form) -eq 'OK') {
            Clear-Build
            $state.checked = $false
            $fw.Text = $dialog.FileName
            Start-Action 'inspect' 'Checking firmware / 正在校验官方固件'
        }
    } finally { $dialog.Dispose() }
})
$selectModules.Add_Click({
    $path = Select-Folder 'Select the folder containing the three verified modules / 选择已保存的模块目录'
    if ($path) { Clear-Build; $modules.Text = $path; Refresh-Controls }
})
$read.Add_Click({
    if ($modules.Text -eq $cache -and [IO.Directory]::Exists($cache)) { [Diagnostics.Process]::Start($cache) | Out-Null; return }
    Clear-Build
    Start-Action 'export' 'Looking for your X6 / 正在查找相机'
})
$cancel.Add_Click({ $state.job.CancelExport(); $cancel.Enabled = $false; $status.Text = 'Canceling export / 正在取消导出' })
$build.Add_Click({
    $path = Select-Folder 'Select a local build output folder / 选择电脑上的输出位置'
    if ($path) { Clear-Build; $state.output = $path; Start-Action 'build' 'Preparing build / 正在准备构建' }
})
$folder.Add_Click({ [Diagnostics.Process]::Start((Join-Path $state.output 'OpenX6 SD Build')) | Out-Null })
$deploy.Add_Click({ Start-Action 'deploy' 'Looking for your X6 / 正在查找相机' })

function Check-Update {
    if (!$env:OPENX6_REPOSITORY -or $state.check) { return }
    try {
        $state.check = New-JobProcess '--desktop-check'
        $updateItem.Text = 'Checking… / 正在检查更新'
    } catch { $updateItem.Text = 'Retry update check / 重试检查更新' }
    Refresh-Controls
}
$updateItem.Visible = [bool]$env:OPENX6_REPOSITORY
$updateItem.Add_Click({
    if ($state.update) { Start-Action 'update' 'Downloading and verifying / 正在下载并校验桌面更新' }
    else { Check-Update }
})
function Update-Status($line) {
    if ($line.StartsWith('OpenX6 verified ')) { return }
    $status.Text = $line
    if ($line -match '(\d{1,3})%\s*$') {
        $bar.Style = 'Continuous'
        $bar.Value = [Math]::Min(100,[int]$Matches[1])
    } else { $bar.Style = 'Marquee' }
}
$timer = New-Object Windows.Forms.Timer
$timer.Interval = 120
$timer.Add_Tick({
    if ($state.check -and $state.check.Finished()) {
        if ($state.check.ExitCode -ne 0) { $updateItem.Text = 'Retry update check / 重试检查更新' }
        elseif ($state.check.LastOutput) {
            $state.update = $state.check.LastOutput.Trim()
            $updateItem.Text = "Update to $($state.update) / 更新桌面工具"
        } else { $updateItem.Text = 'Up to date / 已是最新版' }
        $state.check.Dispose(); $state.check = $null; Refresh-Controls
    }
    $job = $state.job
    if (!$job) { return }
    $finished = $job.Finished()
    while ($null -ne ($line = $job.NextLine())) { Update-Status $line }
    if (!$finished) { return }
    $kind = $state.kind
    $state.job = $null; $state.kind = ''
    try {
        if ($job.ExitCode -ne 0) {
            $status.Text = if ($job.LastError) { $job.LastError } else { 'Operation failed / 操作失败' }
        } elseif ($kind -eq 'inspect') {
            $state.checked = $true; $status.Text = 'Verified / 已验证 X6 1.2.36'
        } elseif ($kind -eq 'export') {
            if ($job.LastOutput -eq $cache -and [IO.Directory]::Exists($cache)) {
                $modules.Text = $cache; $status.Text = 'Modules verified / 三个模块已校验，可以构建'
            } else { $status.Text = 'Canceled / 已取消' }
        } elseif ($kind -eq 'build') {
            if ($job.LastOutput -cmatch '^OpenX6 verified ([0-9a-f]{64})$') {
                $state.digest = $Matches[1]
                $status.Text = 'Verified local copy / 已在电脑上保存并校验'
            } else { $status.Text = 'Missing verified build receipt / 未收到构建验签结果' }
        } elseif ($kind -eq 'deploy') {
            $status.Text = $job.LastOutput
        } elseif ($kind -eq 'update') { $form.Close() }
    } finally { $job.Dispose(); Refresh-Controls }
})
$form.Add_FormClosing({ param($sender,$argsEvent) if ($state.job) { $argsEvent.Cancel = $true } })
$form.Add_Shown({
    if ([IO.Directory]::Exists($cache)) { $modules.Text = $cache }
    Refresh-Controls
    $ready = [Diagnostics.ProcessStartInfo]::new($env:OPENX6_SELF,'--desktop-ready')
    $ready.UseShellExecute = $false
    $ready.CreateNoWindow = $true
    [Diagnostics.Process]::Start($ready).Dispose()
    Check-Update
    $timer.Start()
})
try { [void]$form.ShowDialog() } finally { $timer.Dispose(); $form.Dispose(); if ($state.check) { $state.check.Dispose() } }
