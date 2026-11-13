"""Update visible Tk strings without recreating the window or resetting a build."""
import tkinter as tk
from tkinter import messagebox

from .ui_language import load_language, preference_path, render, save_language


class InterfaceLanguage:
    def __init__(self, root, path=None):
        self.root = root
        self.path = path or preference_path()
        self.code = load_language(self.path)
        self.root.tk.call('::msgcat::mclocale', 'zh_cn' if self.code == 'zh' else 'en')
        self.choice = tk.StringVar(master=root, value=self.code)
        self.bindings = {}

    def text(self, message):
        return render(message, self.code)

    def bind(self, setter, source, key=None):
        self.bindings[key or setter] = (setter, source)
        setter(self.text(source))

    def variable(self, initial=''):
        language = self

        class DisplayText(tk.StringVar):
            def set(self, source):
                language.bind(super().set, source, key=self._name)

        return DisplayText(master=self.root, value=initial)

    def set_widget(self, widget, source):
        self.bind(lambda text: widget.configure(text=text), source, key=widget)

    def set_menu(self, menu, index, source):
        self.bind(lambda text: menu.entryconfigure(index, label=text), source, key=(menu, index))

    def attach(self, container):
        for widget in container.winfo_children():
            if widget not in self.bindings and 'text' in widget.keys() and not widget.cget('textvariable'):
                self.set_widget(widget, widget.cget('text'))

    def add_selector(self, menubar):
        menu = tk.Menu(menubar, tearoff=False)
        menubar.add_cascade(label=self.text('Language'), menu=menu)
        self.set_menu(menubar, menubar.index('end'), 'Language')
        for code, name in [('en', 'English'), ('zh', '简体中文')]:
            menu.add_radiobutton(label=name, variable=self.choice, value=code, command=self.change)

    def change(self):
        code = self.choice.get()
        try:
            save_language(self.path, code)
        except OSError as exc:
            self.choice.set(self.code)
            messagebox.showerror('OpenX6', str(exc), parent=self.root)
            return
        self.code = code
        self.root.tk.call('::msgcat::mclocale', 'zh_cn' if code == 'zh' else 'en')
        for setter, source in self.bindings.values():
            setter(self.text(source))
