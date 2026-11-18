"""Portable profile storage, with import of earlier Windows DPAPI profiles."""
import ctypes
from ctypes import wintypes
from dataclasses import asdict
import json

from csqtt_client.config import PROFILE, LEGACY_PROFILES
from csqtt_client.models.profile import Profile


class Blob(ctypes.Structure):
    _fields_ = [("size", wintypes.DWORD), ("data", ctypes.POINTER(ctypes.c_ubyte))]


def dpapi(value: bytes, decrypt=False):
    """Encrypt for the current Windows account; never write plaintext profiles."""
    buffer = ctypes.create_string_buffer(value)
    source = Blob(len(value), ctypes.cast(buffer, ctypes.POINTER(ctypes.c_ubyte)))
    target = Blob()
    crypt = ctypes.WinDLL("crypt32", use_last_error=True)
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    kernel.LocalFree.argtypes = [ctypes.c_void_p]
    kernel.LocalFree.restype = ctypes.c_void_p
    if decrypt:
        ok = crypt.CryptUnprotectData(ctypes.byref(source), None, None, None, None, 1, ctypes.byref(target))
    else:
        ok = crypt.CryptProtectData(ctypes.byref(source), "CSQTT profile", None, None, None, 1, ctypes.byref(target))
    if not ok:
        raise ctypes.WinError(ctypes.get_last_error())
    try:
        return ctypes.string_at(target.data, target.size)
    finally:
        kernel.LocalFree(target.data)


def save_profile(profile):
    PROFILE.parent.mkdir(parents=True, exist_ok=True)
    temp = PROFILE.with_suffix(".tmp")
    try:
        temp.write_text(json.dumps(asdict(profile), ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        temp.replace(PROFILE)
    finally:
        temp.unlink(missing_ok=True)


def load_profile():
    if PROFILE.exists():
        data = json.loads(PROFILE.read_text(encoding="utf-8-sig"))
    else:
        source = next((path for path in LEGACY_PROFILES if path.exists()), None)
        if source is None:
            return Profile()
        data = json.loads(dpapi(source.read_bytes(), decrypt=True))
    if not isinstance(data, dict) or any(not isinstance(v, str) for k, v in data.items() if k in Profile.__dataclass_fields__):
        raise ValueError("Неверный формат profile.json.")
    return Profile(**{k: v for k, v in data.items() if k in Profile.__dataclass_fields__})


def profile_exists():
    return PROFILE.exists() or any(path.exists() for path in LEGACY_PROFILES)


def delete_profile():
    PROFILE.unlink(missing_ok=True)
    for path in LEGACY_PROFILES:
        path.unlink(missing_ok=True)
