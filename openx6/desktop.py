"""OpenX6 offline desktop preview."""
import os
import queue
import sys
import threading
import tkinter as tk
from pathlib import Path
from tkinter import filedialog, ttk

from .build import build, outside_repo, preflight_modules, ROOT
from .desktop_keys import create_holder
from .official import inspect
from sdk import choice
from sdk.verify_menu_candidate import verify as verify_menus


class Window:
    def __init__(self, root):
        self.root=root
        from .tk_language import InterfaceLanguage
        self.ui = InterfaceLanguage(root)
        self.ui.bind(root.title, 'OpenX6 · Locked build')
        box=ttk.Frame(root,padding=18)
        box.pack(fill='both',expand=True)
        box.columnconfigure(1,weight=1)
        self.buttons=[]
        self.fields={}
        self.events=queue.Queue()
        self.busy=False
        self.firmware_verified=False
        self.full=tk.BooleanVar()
        self.modules=[]
        ttk.Label(box,text='OpenX6 · X6 1.2.36',font=('Helvetica',19,'bold')).grid(row=0,column=0,columnspan=3,sticky='w')
        self.mode=ttk.Checkbutton(box,text='Full OpenX6 build',variable=self.full,command=self.mode_changed)
        self.mode.grid(row=1,column=0,columnspan=3,sticky='w',pady=6)
        self.extra=[]
        for row,(key,title) in enumerate([('firmware','Official firmware'),('material','X6 module folder'),
                                          ('holder','Holder profile')],2):
            field=self.fields[key]=tk.StringVar()
            widgets=[ttk.Label(box,text=title),ttk.Entry(box,textvariable=field,width=49,state='readonly'),
                     ttk.Button(box,text='Choose',command=lambda k=key:self.choose(k))]
            for col,w in enumerate(widgets):
                w.grid(row=row,column=col,sticky='ew',padx=3,pady=3)
            self.buttons.append(widgets[-1])
            if key == 'holder': self.extra.extend(widgets)
        self.extra.append(self.button(box,'Create holder',self.enroll,6,1))
        ttk.Label(box,text='Modules').grid(row=7,column=0,sticky='w')
        self.listbox=tk.Listbox(box,height=3,selectmode='extended',exportselection=False)
        self.listbox.grid(row=7,column=1,sticky='ew')
        self.button(box,'Add JSON',self.select_modules,7,2)
        self.button(box,'Check modules',self.check_modules,8,1)
        self.button(box,'Remove',self.remove,8,2)
        self.button(box,'Check firmware',self.check,9,0)
        self.build_button=self.button(box,'Build locked candidate',self.submit,9,1)
        self.build_button.state(['disabled'])
        self.status=self.ui.variable()
        ttk.Label(box,textvariable=self.status,wraplength=590).grid(row=10,column=0,columnspan=3,sticky='w',pady=8)
        self.ui.attach(box)
        menu = tk.Menu(root)
        root.configure(menu=menu)
        self.ui.add_selector(menu)
        self.mode_changed()
        root.protocol('WM_DELETE_WINDOW',lambda: self.status.set('Build in progress') if self.busy else root.destroy())
        root.after(100,self.poll)

    def button(self,box,label,callback,row,col):
        b=ttk.Button(box,text=label,command=callback)
        b.grid(row=row,column=col,padx=3,pady=4,sticky='w')
        self.buttons.append(b)
        return b

    def mode_changed(self):
        self.modules.clear()
        self.listbox.delete(0,tk.END)
        for w in self.extra:
            if self.full.get(): w.grid()
            else: w.grid_remove()
        self.status.set('Example: '+('recording_budget/module.json / field_checklist.json / media_debug_label.json' if self.full.get() else 'stock_debug_label.json'))

    def choose(self,key):
        p=filedialog.askdirectory() if key == 'material' else filedialog.askopenfilename()
        if p:
            self.fields[key].set(p)
            if key=='firmware': self.check()

    def select_modules(self):
        for p in filedialog.askopenfilenames(initialdir=ROOT/'sdk/examples',filetypes=[('JSON','*.json')]):
            if Path(p) not in self.modules:
                self.modules.append(Path(p))
                path = Path(p)
                self.listbox.insert(tk.END,str(Path(path.parent.name)/path.name))

    def check_modules(self):
        if not self.modules:
            self.status.set('Add a module to check')
            return
        full = self.full.get()
        paths = tuple(self.modules)
        def job():
            docs, _ = preflight_modules(paths, full=full)
            return 'Modules checked: ' + ', '.join(doc['id'] for doc in docs)
        self.run(job)

    def remove(self):
        for i in reversed(self.listbox.curselection()):
            del self.modules[i]
            self.listbox.delete(i)

    def run(self,job,success='done'):
        self.busy=True
        for w in [self.mode,*self.buttons]:w.state(['disabled'])
        self.status.set('Working')
        def worker():
            try:self.events.put((success,job()))
            except Exception as exc:self.events.put(('error',str(exc)))
        threading.Thread(target=worker,daemon=True).start()

    def poll(self):
        while not self.events.empty():
            kind,value=self.events.get()
            if kind=='progress':self.status.set(value);continue
            self.busy=False
            for w in [self.mode,*self.buttons]:w.state(['!disabled'])
            if kind=='firmware_verified':self.firmware_verified=True
            if not self.firmware_verified:self.build_button.state(['disabled'])
            if isinstance(value,Path):
                self.fields['holder'].set(str(value))
                value='Holder created'
            self.status.set(('Error: ' if kind=='error' else '')+value)
        self.root.after(100,self.poll)

    def enroll(self):
        base=filedialog.askdirectory(title=self.ui.text('Save holder outside the repository'))
        if base:
            target=Path(base)/'OpenX6 Holder'
            def job():
                outside_repo(target)
                return create_holder(target)
            self.run(job)

    def check(self):
        path=Path(self.fields['firmware'].get())
        self.firmware_verified=False
        self.run(lambda:'Verified '+inspect(path)[0]['version'],success='firmware_verified')

    def submit(self):
        if not self.firmware_verified:return
        full=self.full.get()
        keys=['firmware','material']+(['holder'] if full else [])
        if not all(self.fields[k].get() for k in keys):
            self.status.set('Choose the required files');return
        base=filedialog.askdirectory(title=self.ui.text('Output parent'))
        if not base:return
        args={k:Path(self.fields[k].get()) for k in keys}
        modules=tuple(self.modules)
        dest=Path(base)/'OpenX6 Build'
        def job():
            build(**args,manifests=modules,dest=dest,progress=lambda s:self.events.put(('progress',s)))
            if (dest/'menu-build.json').is_file():
                self.events.put(('progress','Reading menus and handlers from the candidate'))
                try:
                    docs, _ = preflight_modules(modules, full=True)
                    menus = [path for path, doc in zip(modules, docs)
                             if doc['schema'] in ('openx6.menu.v1', choice.SCHEMA)]
                    report = verify_menus(dest, menus)
                except Exception as exc:
                    raise ValueError(f'Candidate saved to {dest}. Menu readback failed: {exc}') from exc
                return 'Menu readback OK (' + ', '.join(report['modules']) + '): ' + str(dest)
            return 'Verified: '+str(dest)
        self.run(job)


if __name__=='__main__':
    # Aqua can block inside Tk() on the shared Python app's saved-window prompt.
    # AppKit reads this per-launch default from the native process arguments.
    restore_args = ['-ApplePersistenceIgnoreState', 'YES']
    if sys.platform == 'darwin' and sys.argv[-2:] != restore_args:
        os.execv(sys.executable, [sys.executable, *sys.orig_argv[1:], *restore_args])
    os.umask(0o077)
    r=tk.Tk()
    Window(r)
    r.mainloop()
