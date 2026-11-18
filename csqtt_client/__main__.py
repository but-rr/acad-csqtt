"""Entry point for python -m csqtt_client."""
import ctypes
import sys
import tkinter as tk

from csqtt_client.application import App


def main():
    try:
        ctypes.windll.shcore.SetProcessDpiAwareness(1)
    except (AttributeError, OSError):
        pass
    if sys.platform == "win32":
        ctypes.windll.shell32.SetCurrentProcessExplicitAppUserModelID("CSQTT.Desktop")
    root = tk.Tk()
    smoke = "--smoke" in sys.argv
    ui = App(root, persist=not smoke)
    if sys.platform == "win32":
        ui.start_tray()
    if smoke:
        root.after(700, ui.close)
    else:
        def check_updates():
            ui.check_updates(automatic=True)
            root.after(6 * 60 * 60 * 1000, check_updates)
        root.after(1500, check_updates)
    root.mainloop()


if __name__ == "__main__":
    main()
