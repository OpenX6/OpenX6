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
