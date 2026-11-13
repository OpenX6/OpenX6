"""Exercise the visible Mac workflow and the File menu with real Tk controls."""
import gc
from pathlib import Path
import sys
import tempfile
import tkinter as tk
import unittest
from unittest.mock import Mock, patch

from openx6 import consumer_app


@unittest.skipUnless(sys.platform == 'darwin', 'macOS Tk acceptance')
class ReaderGUI(unittest.TestCase):
    def test_simple_home_and_module_folder_menu(self):
        gc.collect()
        with tempfile.TemporaryDirectory() as home:
            gui = tk.Tk()
            gui.withdraw()
            def exercise():
                frame = gui.winfo_children()[0]
                controls = frame.winfo_children()
                fields = [w for w in controls if w.winfo_class() == 'TEntry']
                visible = [w.cget('text') for w in controls if w.winfo_class() == 'TButton' and w.grid_info()]
                self.assertEqual(len(visible), 3)
                for label in ('Choose', 'Unpack on my X6', 'Build SD package'):
                    self.assertTrue(any(text.startswith(label) for text in visible))
                menubar = gui.nametowidget(gui.cget('menu'))
                file_menu = gui.nametowidget(menubar.entrycget('File', 'menu'))
                file_menu.invoke(0)
                self.assertEqual(fields[1].get(), '/existing/module-cache')
            try:
                with (patch.object(tk, 'Tk', return_value=gui),
                      patch.object(gui, 'mainloop', exercise),
                      patch.object(consumer_app, 'holder_home', return_value=Path(home)/'Holder'),
                      patch('tkinter.filedialog.askdirectory', return_value='/existing/module-cache'),
                      patch.object(sys, 'argv', ['OpenX6'])):
                    consumer_app.main()
            finally:
                for timer in gui.tk.splitlist(gui.tk.call('after', 'info')):
                    gui.after_cancel(timer)
                gui.destroy()
