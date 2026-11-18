"""Request Windows elevation only when a native adapter is needed."""
import ctypes
import subprocess
import sys

from csqtt_client.config import ROOT


def is_admin():
    return bool(ctypes.windll.shell32.IsUserAnAdmin())


def relaunch_for_tun():
    args = ["--connect"] if getattr(sys, "frozen", False) else [str(ROOT / "main.py"), "--connect"]
    shell = ctypes.windll.shell32.ShellExecuteW
    shell.argtypes = [ctypes.c_void_p, ctypes.c_wchar_p, ctypes.c_wchar_p, ctypes.c_wchar_p, ctypes.c_wchar_p, ctypes.c_int]
    shell.restype = ctypes.c_ssize_t
    if shell(None, "runas", sys.executable, subprocess.list2cmdline(args), str(ROOT), 1) <= 32:
        raise OSError("Запуск TUN отменён. Для сетевого адаптера нужны права администратора.")
