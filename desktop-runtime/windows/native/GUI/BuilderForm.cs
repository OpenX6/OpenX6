using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Text.RegularExpressions;
using System.Windows.Forms;

internal sealed class BuilderForm : Form
{
    private readonly string worker = Path.Combine(AppDomain.CurrentDomain.BaseDirectory, "OpenX6.exe");
    private readonly string cache = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),
                                                 "OpenX6", "Camera", "X6-1.2.36-modules");
    private readonly TableLayoutPanel grid = new TableLayoutPanel();
    private readonly TextBox firmware = new TextBox { ReadOnly = true };
    private readonly TextBox modules = new TextBox { ReadOnly = true };
    private readonly Button choose = ButtonWith("Choose");
    private readonly Button read = ButtonWith("");
    private readonly Button build = ButtonWith("Build SD package");
    private readonly Button cancel = ButtonWith("Cancel export");
    private readonly Label info = LabelWith("");
    private readonly Label status = LabelWith("X6 · 1.2.36");
    private readonly ProgressBar progress = new ProgressBar();
    private readonly FlowLayoutPanel actions = new FlowLayoutPanel { AutoSize = true, WrapContents = true };
    private readonly ToolStripMenuItem select = new ToolStripMenuItem("Choose saved module folder…");
    private readonly ToolStripMenuItem update = new ToolStripMenuItem("Check for updates…");
    private readonly Timer timer = new Timer { Interval = 120 };
    private OpenX6Job job;
    private OpenX6Job updateCheck;
    private string kind = "", output = "", digest = "", newer = "";
    private bool verified;
    private readonly DesktopLanguage language = new DesktopLanguage();
    private readonly Dictionary<Control, string> captions = new Dictionary<Control, string>();
    private readonly Dictionary<ToolStripItem, string> menuCaptions = new Dictionary<ToolStripItem, string>();
    private readonly ToolStripMenuItem languageMenu = new ToolStripMenuItem();
    private readonly ToolStripMenuItem english = new ToolStripMenuItem("English");
    private readonly ToolStripMenuItem chinese = new ToolStripMenuItem("简体中文");

    private void Caption(Control control, string text)
    {
        captions[control] = text;
        control.Text = language.Text(text);
    }

    private void Caption(ToolStripItem item, string text)
    {
        menuCaptions[item] = text;
        item.Text = language.Text(text);
    }

    private void ApplyLanguage()
    {
        foreach (var entry in captions) entry.Key.Text = language.Text(entry.Value);
        foreach (var entry in menuCaptions) entry.Key.Text = language.Text(entry.Value);
        english.Checked = !language.IsChinese;
        chinese.Checked = language.IsChinese;
    }

    private void SelectLanguage(string code)
    {
        try { language.Save(code); ApplyLanguage(); }
        catch (Exception ex) { MessageBox.Show(this, ex.Message, "OpenX6"); }
    }

    private static readonly Regex Percent = new Regex(@"(\d{1,3})%\s*$");
    private static readonly Regex Footer = new Regex(@"\AOpenX6 verified ([0-9a-f]{64})\z");

    [STAThread]
    private static void Main()
    {
        Application.EnableVisualStyles();
        Application.SetCompatibleTextRenderingDefault(false);
        using (var form = new BuilderForm()) Application.Run(form);
    }

    private static Button ButtonWith(string text) { return new Button { Text = text, AutoSize = true }; }
    private static Label LabelWith(string text) { return new Label { Text = text, AutoSize = true }; }

    private void Add(Control control, int column, int row, int span = 1)
    {
        control.Anchor = AnchorStyles.Left | AnchorStyles.Right;
        control.Margin = new Padding(4);
        grid.Controls.Add(control, column, row);
        grid.SetColumnSpan(control, span);
    }

    private BuilderForm()
    {
        Caption(this, "OpenX6 · SD Card Builder");
        StartPosition = FormStartPosition.CenterScreen;
        AutoScaleMode = AutoScaleMode.Font;
        Font = new Font("Segoe UI", 10);
        ClientSize = new Size(786, 354);
        MinimumSize = new Size(730, 385);
        var menu = new MenuStrip();
        var file = new ToolStripMenuItem("File");
        menu.Items.Add(file);
        file.DropDownItems.AddRange(new ToolStripItem[] { select, update });
        menu.Items.Add(languageMenu);
        languageMenu.DropDownItems.AddRange(new ToolStripItem[] { english, chinese });
        english.Click += delegate { SelectLanguage("en"); };
        chinese.Click += delegate { SelectLanguage("zh"); };
        Caption(file, "File");
        Caption(languageMenu, "Language");
        Caption(select, select.Text);
        Caption(update, update.Text);
        MainMenuStrip = menu;
        grid.Dock = DockStyle.Fill;
        grid.Padding = new Padding(16);
        grid.ColumnCount = 3;
        grid.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
        grid.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
        grid.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
        Add(LabelWith("For Wi-Fi deployment, insert an SD card with no update files."), 0, 0, 3);
        Add(LabelWith("Official X6 firmware"), 0, 1);
        Add(firmware, 1, 1); Add(choose, 2, 1);
        Add(LabelWith("Build input"), 0, 2);
        Add(modules, 1, 2); Add(read, 2, 2);
        Add(info, 0, 3, 3); Add(cancel, 0, 4); Add(build, 2, 4);
        status.MaximumSize = new Size(725, 72);
        Add(status, 0, 5, 3); Add(progress, 0, 6, 3);
        var folder = ButtonWith("Open output folder");
        var deploy = ButtonWith("Deploy to SD via Wi-Fi");
        actions.Controls.AddRange(new Control[] { folder, deploy });
        Add(actions, 0, 7, 3);
        Controls.Add(grid); Controls.Add(menu);
        foreach (Control item in grid.Controls)
            if (item is Button || item is Label) Caption(item, item.Text);
        Caption(folder, folder.Text);
        Caption(deploy, deploy.Text);
        ApplyLanguage();

        choose.Click += delegate {
            using (var dialog = new OpenFileDialog { Title = language.Text("Select official X6 1.2.36 firmware") }) {
                if (dialog.ShowDialog(this) != DialogResult.OK) return;
                ClearBuild(); verified = false; firmware.Text = dialog.FileName;
                StartJob("inspect", "Checking firmware");
            }
        };
        select.Click += delegate {
            var path = SelectFolder("Select the folder containing the three verified modules");
            if (path == null) return;
            ClearBuild(); modules.Text = path; RefreshButtons();
        };
        read.Click += delegate {
            if (IsCached) { OpenFolder(cache); return; }
            ClearBuild(); StartJob("export", "Looking for your X6");
        };
        build.Click += delegate {
            var path = SelectFolder("Select a local build output folder");
            if (path == null) return;
            ClearBuild(); output = path; StartJob("build", "Preparing build");
        };
        cancel.Click += delegate {
            if (kind != "export" || job == null) return;
            job.CancelExport(); cancel.Enabled = false; Caption(status, "Canceling export");
        };
        folder.Click += delegate { OpenFolder(Path.Combine(output, "OpenX6 SD Build")); };
        deploy.Click += delegate { StartJob("deploy", "Looking for your X6"); };
        update.Visible = !String.IsNullOrEmpty(Environment.GetEnvironmentVariable("OPENX6_REPOSITORY"));
        update.Click += delegate {
            if (newer.Length == 0) CheckUpdate();
            else StartJob("update", "Downloading and verifying");
        };
        timer.Tick += delegate { Poll(); };
        FormClosing += delegate(object sender, FormClosingEventArgs args) { if (job != null) args.Cancel = true; };
    }

    protected override void OnShown(EventArgs e)
    {
        base.OnShown(e);
        if (Directory.Exists(cache)) modules.Text = cache;
        RefreshButtons();
        try {
            using (Process.Start(new ProcessStartInfo(worker, "--desktop-ready") { UseShellExecute = false, CreateNoWindow = true })) { }
        } catch (Exception ex) { Caption(status, ex.Message); }
        CheckUpdate();
        timer.Start();
    }

    protected override void Dispose(bool disposing)
    {
        if (disposing) { timer.Dispose(); if (updateCheck != null) updateCheck.Dispose(); }
        base.Dispose(disposing);
    }

    private bool IsCached { get { return String.Equals(modules.Text, cache, StringComparison.OrdinalIgnoreCase) && Directory.Exists(cache); } }

    private string SelectFolder(string description)
    {
        using (var dialog = new FolderBrowserDialog { Description = language.Text(description) })
            return dialog.ShowDialog(this) == DialogResult.OK ? dialog.SelectedPath : null;
    }

    private void OpenFolder(string path)
    {
        try { using (Process.Start(new ProcessStartInfo(path) { UseShellExecute = true })) { } }
        catch (Exception ex) { Caption(status, ex.Message); }
    }

    private void ClearBuild() { output = ""; digest = ""; actions.Visible = false; }

    private void RefreshButtons()
    {
        bool idle = job == null, cached = IsCached;
        languageMenu.Enabled = idle;
        choose.Enabled = select.Enabled = idle;
        read.Enabled = idle && (cached || verified);
        Caption(read, cached ? "Open cache folder" : "Unpack on my X6");
        if (cached) Caption(info, "Saved on this PC · ready for offline builds");
        else if (modules.TextLength != 0) Caption(info, "Selected module folder");
        else Caption(info, "First use: connect to X6 Wi-Fi to unpack");
        build.Enabled = idle && verified && modules.TextLength != 0;
        actions.Visible = digest.Length != 0; actions.Enabled = idle;
        cancel.Visible = kind == "export"; progress.Visible = !idle; update.Enabled = idle && updateCheck == null;
    }

    private OpenX6Job NewJob(string arg)
    {
        return new OpenX6Job(worker, arg, new Dictionary<string, string> {
            { "OPENX6_UI_LANG", language.IsChinese ? "zh" : "en" }, { "OPENX6_FW", firmware.Text }, { "OPENX6_MODULES", modules.Text }, { "OPENX6_OUTPUT", output },
            { "OPENX6_DIGEST", digest }, { "OPENX6_UPDATE", newer }
        });
    }

    private void StartJob(string action, string message)
    {
        if (job != null) return;
        try {
            job = NewJob("--ui-" + action); kind = action; Caption(status, message);
            progress.Style = ProgressBarStyle.Marquee; cancel.Enabled = true;
        } catch (Exception ex) { Caption(status, ex.Message); }
        RefreshButtons();
    }

    private void CheckUpdate()
    {
        if (!update.Available || updateCheck != null) return;
        try { updateCheck = NewJob("--desktop-check"); Caption(update, "Checking…"); }
        catch (Exception) { Caption(update, "Retry update check"); }
        RefreshButtons();
    }

    private void Poll()
    {
        if (updateCheck != null && updateCheck.Finished()) {
            if (updateCheck.ExitCode != 0) Caption(update, "Retry update check");
            else {
                newer = (updateCheck.LastOutput ?? "").Trim();
                Caption(update, newer.Length == 0 ? "Up to date" : "Update to " + newer);
            }
            updateCheck.Dispose(); updateCheck = null; RefreshButtons();
        }
        if (job == null) return;
        bool finished = job.Finished();
        string line;
        while ((line = job.NextLine()) != null) {
            if (line.StartsWith("OpenX6 verified ", StringComparison.Ordinal)) continue;
            Caption(status, line);
            var percent = Percent.Match(line);
            progress.Style = percent.Success ? ProgressBarStyle.Continuous : ProgressBarStyle.Marquee;
            if (percent.Success) progress.Value = Math.Min(100, Int32.Parse(percent.Groups[1].Value));
        }
        if (!finished) return;
        var completed = job; string action = kind;
        job = null; kind = "";
        using (completed) {
            if (completed.ExitCode != 0) Caption(status, completed.LastError ?? "Operation failed");
            else if (action == "inspect") { verified = true; Caption(status, "Verified X6 1.2.36"); }
            else if (action == "export") {
                if (completed.LastOutput == cache && Directory.Exists(cache)) {
                    modules.Text = cache; Caption(status, "Modules verified");
                } else Caption(status, "Canceled");
            } else if (action == "build") {
                var match = Footer.Match(completed.LastOutput ?? "");
                if (match.Success) { digest = match.Groups[1].Value; Caption(status, "Verified local copy"); }
                else Caption(status, "Missing verified build receipt");
            } else if (action == "deploy") Caption(status, completed.LastOutput ?? "");
            else if (action == "update") Close();
        }
        if (!IsDisposed) RefreshButtons();
    }
}
