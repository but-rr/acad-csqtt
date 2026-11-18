"""Application identity and paths shared by source and portable builds."""
import os
from pathlib import Path
import subprocess
import sys

APP_NAME = "CSQTT"
ROOT = Path(sys.executable).parent if getattr(sys, "frozen", False) else Path(__file__).resolve().parents[1]
RUNTIME = Path(getattr(sys, "_MEIPASS", ROOT))
APP_VERSION = (RUNTIME / "VERSION").read_text(encoding="utf-8").strip()
ASSETS = ROOT / "assets"
PROFILE = ROOT / "profile.json"
# Import encrypted profiles created by earlier versions on this Windows account.
_local_data = Path(os.environ.get("LOCALAPPDATA", str(ROOT)))
LEGACY_PROFILES = tuple(_local_data / name / "profile.dat" for name in (APP_NAME, "CSQTT-Kolenka"))
NO_WINDOW = getattr(subprocess, "CREATE_NO_WINDOW", 0)
