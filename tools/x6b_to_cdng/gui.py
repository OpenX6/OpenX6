#!/usr/bin/env python3
"""Small optional Tk folder picker; the command-line converter also works alone."""
import queue
import threading
import tkinter as tk
from tkinter import filedialog, messagebox, ttk
from pathlib import Path

from convert import convert


def main():
    root = tk.Tk()
    root.title('OpenX6 — X6B to CinemaDNG')
    root.geometry('760x410')
    root.minsize(640, 390)
    outer = ttk.Frame(root, padding=18); outer.pack(fill='both', expand=True)
    outer.columnconfigure(1, weight=1)
    source, dest = tk.StringVar(), tk.StringVar()
    eye, gaps = tk.StringVar(value='both'), tk.StringVar(value='hold')
    status = tk.StringVar(value='Choose a completed RAWVIDEO take. Original files are never changed.')
    stop = threading.Event()
    events = queue.Queue()
    running = False

    def choose_source():
        selected = filedialog.askdirectory(title='Select RAWVIDEO folder')
        if selected:
            source.set(selected)
            dest.set(str(Path(selected).with_name(Path(selected).name+'_CDNG')))

    def choose_output():
        selected = filedialog.askdirectory(title='Select output parent folder')
        if selected:
            dest.set(str(Path(selected)/((Path(source.get()).name or 'RAWVIDEO')+'_CDNG')))

    for row, (label, var, command) in enumerate([('Source take', source, choose_source), ('New output folder', dest, choose_output)]):
        ttk.Label(outer, text=label).grid(row=row, column=0, sticky='w', pady=8)
        ttk.Entry(outer, textvariable=var).grid(row=row, column=1, sticky='ew', padx=10)
        ttk.Button(outer, text='Browse…', command=command).grid(row=row, column=2)
    ttk.Label(outer, text='Eye / channel').grid(row=2, column=0, sticky='w', pady=8)
    ttk.Combobox(outer, textvariable=eye, values=('both', 'CAM0', 'CAM1'), state='readonly').grid(row=2, column=1, sticky='w', padx=10)
    ttk.Label(outer, text='Missing frames').grid(row=3, column=0, sticky='w', pady=8)
    ttk.Combobox(outer, textvariable=gaps, values=('hold', 'keep', 'error'), state='readonly').grid(row=3, column=1, sticky='w', padx=10)
    ttk.Label(outer, text='hold = repeat previous frame and log it; keep = leave numbered gaps; error = stop.\n'
              'Uncompressed 16-bit DNGs need more space than X6B. Audio and metadata are copied.\n'
              'First use may pause while Python prepares its decoder cache.', wraplength=690).grid(row=4, column=0, columnspan=3, sticky='w', pady=16)
    ttk.Label(outer, textvariable=status, wraplength=690).grid(row=5, column=0, columnspan=3, sticky='w', pady=12)

    def run():
        nonlocal running
        if running:
            return
        if not source.get() or not dest.get():
            messagebox.showerror('Choose folders', 'Select the source and a new output folder.')
            return
        args = (source.get(), dest.get(), eye.get(), gaps.get())
        running = True; stop.clear(); start.configure(state='disabled'); cancel.configure(state='normal')
        def worker():
            try:
                convert(args[0], args[1], eye=args[2], gaps=args[3],
                        progress=lambda msg: events.put(('status', msg)), cancelled=stop.is_set)
                events.put(('done', 'Conversion complete. See conversion.json and frames.csv.'))
            except Exception as exc:
                events.put(('error', str(exc)))
        threading.Thread(target=worker, daemon=False).start()

    start = ttk.Button(outer, text='Convert', command=run)
    start.grid(row=6, column=1, sticky='e', pady=12)
    cancel = ttk.Button(outer, text='Cancel', state='disabled', command=stop.set)
    cancel.grid(row=6, column=2, padx=(10, 0))

    def poll():
        nonlocal running
        try:
            while True:
                kind, msg = events.get_nowait()
                status.set(msg)
                if kind != 'status':
                    running = False; start.configure(state='normal'); cancel.configure(state='disabled')
                    if kind == 'error':
                        messagebox.showerror('Conversion stopped', msg)
        except queue.Empty:
            pass
        root.after(100, poll)

    def close():
        if running:
            stop.set()
            status.set('Cancelling after the current frame; close again once stopped.')
        else:
            root.destroy()
    root.protocol('WM_DELETE_WINDOW', close)
    root.after(100, poll)
    root.mainloop()


if __name__ == '__main__':
    main()
