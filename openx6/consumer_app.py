"""Consumer UI and the identical headless build entry point used by packaging checks."""
import argparse
import os
import sys
from pathlib import Path


def configure_runtime(runtime):
    tools = runtime/'tools'
    if not tools.is_dir():
        raise ValueError('OpenX6 build runtime is missing.')
    os.environ.update(PATH=os.pathsep.join([str(tools/'bin'),str(tools/'go/bin'),'/usr/bin','/bin']),
                      OPENX6_TOOL_BIN=str(tools/'bin'), GOROOT=str(tools/'go'), ZIG_LIB_DIR=str(tools/'zig-lib'),
                      OPENX6_LIBCRYPTO=str(tools/'libcrypto'), OPENX6_LLD=str(tools/'bin/lld'),
                      GOTOOLCHAIN='local',GOPROXY='off',GOENV='off',GOSUMDB='off')
    for key in ('GOFLAGS','GOOS','GOARCH','GOWORK','CC','CXX'):
        os.environ.pop(key, None)


def holder_home():
    if sys.platform == 'darwin':
        return Path.home()/'Library/Application Support/OpenX6/Holder'
    return Path.home()/'.local/share/OpenX6/Holder'


def deployment_message(result):
    if result == 'SCAN_REQUESTED':
        return 'Verified on SD. Scan requested. Keep power and card connected and watch the camera for the update.'
    prefix = 'Verified on SD. Scan unconfirmed.' if result == 'SCAN_UNCONFIRMED' else 'Verified on SD.'
    return (prefix, 'If still idle, restart with the card inserted. At the charging screen, press Power once.')



def main():
    os.umask(0o077)
    if getattr(sys, 'frozen', False):
        configure_runtime(Path(sys.executable).resolve().parents[1]/'Resources')
    parser = argparse.ArgumentParser()
    parser.add_argument('--build', nargs=3, metavar=('FIRMWARE','MODULE_FOLDER','OUTPUT'))
    parser.add_argument('--existing-holder', type=Path,
                        help='For --build, sign with an existing local Holder directory')
    args = parser.parse_args()
    if args.existing_holder is not None and (not args.build or not args.existing_holder.is_dir()):
        parser.error('--existing-holder requires --build and an existing Holder directory')
    from .consumer import build_sd
    if args.build:
        import json
        print(json.dumps(build_sd(*(Path(p).resolve() for p in args.build), args.existing_holder or holder_home(),
                                  progress=lambda s: print(s,file=sys.stderr,flush=True))))
        return
    import tkinter as tk
    from tkinter import filedialog, messagebox, ttk
    from camera_reader.power import NOTICE
    from camera_reader import export as camera_export, deploy as sd_deploy
    from queue import Empty, Queue
    from threading import Thread
    from .official import inspect
    import json
    import subprocess
    executable = Path(sys.executable).resolve()
    helper = executable.with_name('openx6-update')
    stamp = executable.parents[1]/'Resources/desktop-release.json'
    channel = None
    if sys.platform == 'darwin' and getattr(sys, 'frozen', False) and helper.is_file() and stamp.is_file():
        channel = json.loads(stamp.read_bytes())
    def open_updated(target):
        # Launch Services handles Gatekeeper and the bundle's normal GUI startup.
        app = Path(target).parents[2]
        subprocess.run(['/usr/bin/open', '-n', str(app)], check=True)
    if channel and channel.get('repository'):
        active = subprocess.run([helper, 'active'], capture_output=True, text=True)
        if active.returncode == 0 and active.stdout:
            try:
                open_updated(active.stdout)
                return
            except (OSError, subprocess.SubprocessError):
                pass
    root = tk.Tk()
    from .tk_language import InterfaceLanguage
    ui = InterfaceLanguage(root, holder_home().parent / 'ui-language')
    ui.bind(root.title, 'OpenX6 · SD Card Builder')
    box = ttk.Frame(root, padding=18)
    box.pack(fill='both',expand=True)
    fields = [tk.StringVar(),tk.StringVar()]
    status = ui.variable('X6 · 1.2.36')
    events = Queue()
    cache_parent = holder_home().parent/'Camera'
    cache = cache_parent/camera_export.CACHE
    if cache.is_dir():
        fields[1].set(str(cache))
    built = None
    box.columnconfigure(1, weight=1)
    info = ui.variable()
    def refresh_cache():
        cached = fields[1].get() == str(cache) and cache.is_dir()
        if cached:
            info.set('Saved on this Mac · ready for offline builds')
        elif fields[1].get():
            info.set('Selected module folder')
        else:
            info.set('First use: connect to X6 Wi-Fi to unpack')
        ui.set_widget(read_button, 'Open cache folder' if cached else 'Unpack on my X6')
    def clear_result():
        nonlocal built
        built = None
        folder_button.grid_remove()
        deploy_button.grid_remove()
    cancel_export = None
    busy = False
    buttons = []
    menubar = tk.Menu(root)
    file_menu = tk.Menu(menubar, tearoff=False)
    menubar.add_cascade(label='File', menu=file_menu)
    root.configure(menu=menubar)
    def on_bytes(label, completed, total):
        events.put(('bytes', (label, completed, total)))
    def run(job, finish):
        nonlocal busy
        busy = True
        file_menu.entryconfigure(0, state='disabled')
        bar.grid()
        bar.configure(mode='indeterminate')
        bar.start(90)
        for button in buttons:
            button.state(['disabled'])
        def worker():
            try:
                result = job()
                events.put(('done', lambda: finish(result)))
            except Exception as exc:
                events.put(('error', str(exc)))
        Thread(target=worker, daemon=True).start()
    def poll():
        nonlocal busy
        try:
            while True:
                kind, value = events.get_nowait()
                if kind == 'update-check':
                    ui.set_menu(file_menu, update_index, value[0])
                    file_menu.entryconfigure(update_index, command=value[1])
                    continue
                if kind in ('done', 'error'):
                    stop_button.grid_remove()
                    file_menu.entryconfigure(0, state='normal')
                    bar.stop()
                    bar.grid_remove()
                    busy = False
                    for b in buttons: b.state(['!disabled'])
                if kind == 'bytes':
                    label, completed, total = value
                    bar.stop()
                    bar.configure(mode='determinate', maximum=total, value=completed)
                    status.set(f'{label} · {100*completed//total}%')
                    continue
                if kind == 'progress' and str(bar.cget('mode')) == 'determinate':
                    bar.configure(mode='indeterminate')
                    bar.start(90)
                # Apply completion callbacks on Tk's main thread.
                text = value() if kind == 'done' else value
                if text is not None:
                    status.set(text)
        except Empty:
            pass
        root.after(100,poll)
    def choose(i):
        path = filedialog.askopenfilename(title=ui.text('Select official X6 firmware')) if i == 0 else filedialog.askdirectory(title=ui.text('Select verified X6 module folder'))
        if path:
            clear_result()
            fields[i].set(path)
            refresh_cache()
            if i == 0:
                status.set('Checking firmware')
                run(lambda:inspect(Path(path))[0]['version'],lambda v:'Verified '+v)
    ttk.Label(box, text='For Wi-Fi deployment, insert an SD card with no update files.').grid(row=0,column=0,columnspan=3,sticky='w',pady=(0,10))
    for i, label in enumerate(('Official X6 firmware','Build input')):
        ttk.Label(box,text=label).grid(row=i+1,column=0,sticky='w')
        ttk.Entry(box,textvariable=fields[i],width=42,state='readonly').grid(row=i+1,column=1,padx=(8,10),pady=4,sticky='ew')
        if i == 0:
            b = ttk.Button(box, text='Choose', command=lambda: choose(0))
            b.grid(row=1, column=2, sticky='ew')
            buttons.append(b)
    file_menu.add_command(label='Choose saved module folder…', command=lambda: choose(1))
    def read_camera():
        if fields[1].get() == str(cache) and cache.is_dir():
            subprocess.run(['/usr/bin/open', str(cache)], check=True)
            return
        if not fields[0].get():
            status.set('Choose official X6 1.2.36 first')
            return
        firmware = Path(fields[0].get())
        if cache.is_dir():
            def selected_cache(path):
                clear_result()
                fields[1].set(str(path))
                refresh_cache()
                return 'Verified cached modules'
            run(lambda:camera_export.verify_modules(cache), selected_cache)
            return
        status.set('Looking for your X6')
        def confirmed(identity):
            serial, version = identity
            nonlocal cancel_export
            if version.lower().removeprefix('v') not in camera_export.UPDATERS:
                return 'On-camera unpack requires stock 0.8.825 or 0.8.827.'
            message = ui.text('Match this serial suffix to your camera. Keep it on and connected. Tap Allow on the camera if prompted. OpenX6 will save three verified modules on this computer.')
            prompt = f'Insta360 X6 · {version} · SN …{serial[-6:]}\n\n' + message + '\n\n' + ui.text(NOTICE.splitlines()[0])
            if not messagebox.askokcancel(ui.text('Unpack on my X6'), prompt, parent=root, default='cancel'):
                return 'Canceled'
            from threading import Event
            cancel_export = Event()
            stop_button.grid()
            status.set('Waiting for camera')
            def selected(path):
                clear_result()
                fields[1].set(str(path))
                refresh_cache()
                return 'Modules verified'
            run(lambda:camera_export.export_to_cache(firmware, cache_parent, serial,
                lambda text: events.put(('progress', text)), cancel_export, byte_progress=on_bytes), selected)

        def discover():
            try:
                return camera_export.discover()
            except (OSError, EOFError) as exc:
                raise RuntimeError('Connect your Mac to the X6 Wi-Fi and allow Local Network access.') from exc
        run(discover, confirmed)

    read_button = ttk.Button(box, text='Unpack on my X6', command=read_camera)
    read_button.grid(row=2, column=2, sticky='ew')
    ttk.Label(box,textvariable=info,wraplength=650).grid(row=3,column=0,columnspan=3,sticky='w',pady=(0,8))
    refresh_cache()
    buttons.append(read_button)
    stop_button = ttk.Button(box, text='Cancel export',
                             command=lambda: cancel_export.set() if cancel_export else None)
    stop_button.grid(row=4,column=0,sticky='w')
    stop_button.grid_remove()
    def submit():
        if not all(f.get() for f in fields):
            status.set('Choose firmware and a build input')
            return
        parent = filedialog.askdirectory(title=ui.text('Build on the computer'))
        if not parent: return
        firmware, material = [Path(f.get()) for f in fields]
        dest = Path(parent)/'OpenX6 SD Build'
        clear_result()
        def completed(receipt):
            nonlocal built
            built = dest/receipt['candidate'], receipt
            folder_button.grid()
            deploy_button.grid()
            return 'Verified local copy: ' + str(built[0])
        run(lambda:build_sd(firmware,material,dest,holder_home(),
                            progress=lambda s:events.put(('progress',s)), byte_progress=on_bytes),
            completed)
    b=ttk.Button(box,text='Build SD package',command=submit)
    b.grid(row=4,column=2,pady=6,sticky='ew')
    buttons.append(b)
    def open_output():
        if built:
            subprocess.run(['/usr/bin/open', str(built[0].parent)], check=True)
    def deploy():
        if not built:
            return
        path, receipt = built
        def confirm(identity):
            serial, version = identity
            message = ui.text('Check the serial on your X6. Keep power connected, the camera idle and the SD card inserted. Publishing the verified file can start installation. The copy on your computer is kept.')
            if not messagebox.askokcancel(ui.text('Deploy over Wi-Fi'), f'X6 · {version} · SN …{serial[-6:]}\n\n' + message,
                                         parent=root, default='cancel'):
                return 'Canceled'
            run(lambda: sd_deploy.to_card(path, receipt, identity, lambda s: events.put(('progress',s)), on_bytes),
                deployment_message)
        def discover_deploy():
            try:
                return sd_deploy.discover()
            except (OSError, EOFError) as exc:
                raise ValueError('Connect to camera Wi-Fi and retry') from exc
        status.set('Looking for your X6')
        run(discover_deploy, confirm)
    folder_button = ttk.Button(box,text='Open output folder',command=open_output)
    deploy_button = ttk.Button(box,text='Deploy to SD via Wi-Fi',command=deploy)
    for row, button in enumerate((folder_button, deploy_button), start=7):
        button.grid(row=row,column=2,sticky='ew',pady=2)
        button.grid_remove()
        buttons.append(button)
    ttk.Label(box,textvariable=status,wraplength=580).grid(row=5,column=0,columnspan=3,sticky='w')
    bar = ttk.Progressbar(box, mode='indeterminate')
    bar.grid(row=6, column=0, columnspan=3, sticky='ew', pady=6)
    bar.grid_remove()
    if channel and channel.get('repository'):
        file_menu.add_separator()
        file_menu.add_command(label='Check for updates…')
        update_index = file_menu.index('end')
        def check_update():
            ui.set_menu(file_menu, update_index, 'Checking…')
            file_menu.entryconfigure(update_index, command=lambda: None)
            def worker():
                result = subprocess.run([helper, 'check'], capture_output=True, text=True)
                version = result.stdout.strip()
                if result.returncode:
                    label, command = 'Retry update check', check_update
                elif not version:
                    label, command = 'Up to date', check_update
                else:
                    def apply():
                        status.set('Downloading and verifying OpenX6 Desktop')
                        def install():
                            r = subprocess.run([helper, 'install', version], capture_output=True, text=True)
                            if r.returncode:
                                raise ValueError(r.stderr.strip() or 'Desktop update failed')
                            return r.stdout
                        def reopen(target):
                            try:
                                open_updated(target)
                            except (OSError, subprocess.SubprocessError) as exc:
                                return str(exc)
                            root.after_idle(root.destroy)
                        run(install, reopen)
                    label, command = 'Update to '+version, apply
                events.put(('update-check', (label, command)))
            Thread(target=worker, daemon=True).start()
        root.after_idle(lambda: subprocess.run([helper, 'ready', executable], check=False))
        root.after_idle(check_update)
    ui.attach(box)
    ui.set_menu(menubar, menubar.index('end'), 'File')
    ui.set_menu(file_menu, 0, 'Choose saved module folder…')
    if channel and channel.get('repository'):
        ui.set_menu(file_menu, update_index, 'Check for updates…')
    ui.add_selector(menubar)
    root.protocol('WM_DELETE_WINDOW',lambda:None if busy else root.destroy())
    root.after(100,poll)
    root.mainloop()


if __name__=='__main__':
    main()
