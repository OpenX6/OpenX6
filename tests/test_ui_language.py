"""Exercise persistence, progress and language changes using the real Tk menus."""
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch
import tkinter as tk

from openx6 import consumer_app
from openx6.ui_language import CHINESE, WORKER_MESSAGES, load_language, render, save_language


class Language(unittest.TestCase):
    def test_language_preference_and_worker_progress(self):
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / 'OpenX6/ui-language'
            self.assertEqual(load_language(path), 'en')
            save_language(path, 'zh')
            self.assertEqual(load_language(path), 'zh')
            path.write_bytes(b'unknown')
            self.assertEqual(load_language(path), 'en')
            with self.assertRaises(ValueError):
                save_language(path, 'other')
        for message, english in WORKER_MESSAGES.items():
            self.assertEqual(render(message, 'en'), english)
            self.assertEqual(render(message, 'zh'), CHINESE[english])
        text = '5/6 · Compressing system image / 压缩系统镜像 · 40%'
        self.assertEqual(render(text, 'zh'), '5/6 · 压缩系统镜像 · 40%')
        path = '/sample/相片 / Photos/file.bin'
        self.assertEqual(render('Verified local copy: ' + path, 'zh'), '已在电脑上保存并校验：' + path)
        for code in ('en', 'zh'):
            msg = render(consumer_app.deployment_message('SCAN_UNCONFIRMED'), code)
            self.assertIn('press Power' if code == 'en' else '电源键', msg)

    @unittest.skipUnless(sys.platform == 'darwin', 'macOS Tk')
    def test_selected_material_and_status_survive_switch_and_next_launch(self):
        with tempfile.TemporaryDirectory() as d:
            home = Path(d)
            def launch(initial, selected):
                root = tk.Tk()
                root.withdraw()
                def exercise():
                    menu = root.nametowidget(root.cget('menu'))
                    file = root.nametowidget(menu.entrycget('File' if initial == 'Language' else '文件', 'menu'))
                    language = root.nametowidget(menu.entrycget(initial, 'menu'))
                    file.invoke(0)
                    language.invoke(selected)
                    widgets = root.winfo_children()[0].winfo_children()
                    text = [w.cget('text') for w in widgets if w.winfo_class() == 'TButton']
                    self.assertIn('生成卡刷包' if selected else 'Build SD package', text)
                    self.assertEqual([w for w in widgets if w.winfo_class() == 'TEntry'][1].get(), '/camera/材料')
                    self.assertIn('已选择模块目录' if selected else 'Selected module folder',
                                  [root.getvar(w.cget('textvariable')) for w in widgets
                                   if w.winfo_class() == 'TLabel' and w.cget('textvariable')])
                try:
                    with (patch.object(tk, 'Tk', return_value=root), patch.object(root, 'mainloop', exercise),
                          patch.object(consumer_app, 'holder_home', return_value=home/'Holder'),
                          patch.object(sys, 'argv', ['OpenX6']),
                          patch('tkinter.filedialog.askdirectory', return_value='/camera/材料')):
                        consumer_app.main()
                finally:
                    for timer in root.tk.splitlist(root.tk.call('after', 'info')):
                        root.after_cancel(timer)
                    root.destroy()
            launch('Language', 1)
            launch('语言', 0)
            self.assertEqual((home/'ui-language').read_bytes(), b'en')
            self.assertFalse((home/'Holder').exists())
