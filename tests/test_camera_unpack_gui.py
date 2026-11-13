"""Drive the normal Tk controls through confirmation, export and build selection."""
import gc
from pathlib import Path
import sys
import tempfile
import time
import threading
import tkinter as tk
import unittest
from unittest.mock import Mock, patch

from camera_reader import export
from openx6 import consumer, consumer_app


@unittest.skipUnless(sys.platform == 'darwin', 'macOS Tk')
class CameraUnpackGUI(unittest.TestCase):
    def test_export_then_build(self):
        gc.collect()
        with tempfile.TemporaryDirectory() as d:
            home=Path(d)
            firmware=home/'stock.bin'
            modules=home/'Camera'/export.CACHE
            gui=tk.Tk()
            gui.withdraw()
            read=Mock(return_value=modules)
            release_build = threading.Event()
            def build_with_progress(*args, progress, byte_progress):
                progress('5/6 · Packing')
                byte_progress('Compressing', 7, 20)
                if not release_build.wait(5):
                    raise RuntimeError('GUI missed the progress update')
                progress('6/6 · Signing')
                return {'candidate':'Insta360X6FW.bin'}
            build=Mock(side_effect=build_with_progress)
            def exercise():
                children=gui.winfo_children()[0].winfo_children()
                def button(prefix):
                    return next(w for w in children if w.winfo_class()=='TButton' and w.cget('text').startswith(prefix))
                fields=[w for w in children if w.winfo_class()=='TEntry']
                def idle():
                    deadline=time.monotonic()+4
                    while button('Build SD').instate(['disabled']) and time.monotonic()<deadline:
                        gui.update()
                        time.sleep(.01)
                    self.assertFalse(button('Build SD').instate(['disabled']))
                gui.setvar(fields[0].cget('textvariable'),str(firmware))
                button('Unpack on my X6').invoke()
                idle()
                self.assertEqual(fields[1].get(),str(modules))
                read.assert_called_once()
                self.assertEqual(read.call_args.args[:3],(firmware,home/'Camera','EXAMPLE123456'))
                button('Build SD').invoke()
                bar = next(w for w in children if w.winfo_class()=='TProgressbar')
                deadline = time.monotonic()+4
                while str(bar.cget('mode')) != 'determinate' and time.monotonic() < deadline:
                    gui.update()
                    time.sleep(.01)
                try:
                    self.assertTrue(bar.grid_info())
                    self.assertEqual(float(bar.cget('value'))/float(bar.cget('maximum')), .35)
                finally:
                    release_build.set()
                idle()
                self.assertFalse(bar.grid_info())
                folder, deploy_button = button('Open output folder'), button('Deploy to SD')
                self.assertTrue(folder.grid_info() and deploy_button.grid_info())
                with patch('subprocess.run') as opened:
                    folder.invoke()
                    self.assertEqual(opened.call_args.args[0], ['/usr/bin/open', str(home/'OpenX6 SD Build')])
                with (patch('camera_reader.deploy.discover',return_value=('EXAMPLE123456','v0.8.825')),
                      patch('camera_reader.deploy.to_card') as transfer):
                    deploy_button.invoke()
                    idle()
                    self.assertEqual(transfer.call_args.args[0],home/'OpenX6 SD Build'/'Insta360X6FW.bin')
                    self.assertEqual(transfer.call_args.args[2],('EXAMPLE123456','v0.8.825'))
                    self.assertTrue(folder.grid_info())
                self.assertEqual(build.call_args.args,(firmware,modules,home/'OpenX6 SD Build',home/'Holder'))
            try:
                with (patch.object(tk,'Tk',return_value=gui), patch.object(gui,'mainloop',exercise),
                      patch.object(consumer_app,'holder_home',return_value=home/'Holder'),
                      patch.object(consumer,'build_sd',build), patch.object(export,'export_to_cache',read),
                      patch.object(export,'discover',return_value=('EXAMPLE123456','v0.8.827')),
                      patch('tkinter.messagebox.askokcancel',return_value=True),
                      patch('tkinter.filedialog.askdirectory',return_value=str(home)),
                      patch.object(sys,'argv',['OpenX6'])):
                    consumer_app.main()
            finally:
                for timer in gui.tk.splitlist(gui.tk.call('after','info')): gui.after_cancel(timer)
                gui.destroy()
