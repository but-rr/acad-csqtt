"""Collect required notices into one file inside the runtime directory."""
import json
import importlib.metadata
import os
from pathlib import Path
import shutil
import subprocess
import sys

root = Path(__file__).resolve().parents[1]
destination = Path(sys.argv[1]) / "_internal/THIRD_PARTY_NOTICES.txt"
notices = []


def include_file(path, name):
    data = path.read_bytes()
    try:
        text = data.decode("utf-8-sig")
    except UnicodeDecodeError:
        text = data.decode("latin-1")
    notices.append(f"\n{'=' * 72}\n{name}/{path.name}\n{'=' * 72}\n\n{text.rstrip()}\n")


def copy_notices(source, name):
    if not source.is_dir():
        return
    for path in source.iterdir():
        if path.is_file() and path.name.upper().startswith(("LICENSE", "LICENCE", "COPYING", "NOTICE", "AUTHORS", "COPYRIGHT")):
            include_file(path, name)


include_file(root / "LICENSE", "CSQTT")
include_file(root / "NOTICE.md", "CSQTT")
include_file(root / "assets/fonts/OFL.txt", "Nunito Sans")
include_file(root / "build/wintun/wintun/LICENSE.txt", "Wintun 0.14.1")
copy_notices(Path(sys.base_prefix), "Python")
for distribution in ("pyinstaller", "pystray", "Pillow", "six"):
    for item in importlib.metadata.files(distribution) or []:
        path = Path(item.locate())
        if path.is_file() and "dist-info" in str(item):
            if path.name.upper().startswith(("LICENSE", "LICENCE", "COPYING", "NOTICE")):
                copy_notices(path.parent, distribution)
for tcl in (Path(sys.base_prefix) / "tcl").glob("*"):
    copy_notices(tcl, "TclTk-" + tcl.name)
copy_notices(root / "core", "CSQTT")
copy_notices(root / "core/rust-client/vendor/primp", "primp-vendored")
copy_notices(root / "core/rust-client/vendor/netstack-smoltcp", "netstack-smoltcp-vendored")
cargo_home = Path(os.environ.get("CARGO_HOME", str(Path.home() / ".cargo")))
for registry in (cargo_home / "registry/src").glob("*"):
    for crate in registry.iterdir():
        copy_notices(crate, "rust-" + crate.name)
go = shutil.which("go") or r"C:\Go\bin\go.exe"
modules = subprocess.check_output([go, "list", "-m", "-json", "all"], cwd=root / "packet-proxy", text=True)
decoder = json.JSONDecoder()
while modules.strip():
    module, end = decoder.raw_decode(modules.lstrip())
    modules = modules.lstrip()[end:]
    if "Dir" in module:
        copy_notices(Path(module["Dir"]), "go-" + module["Path"].replace("/", "_") + "-" + module.get("Version", "local"))
destination.write_text("CSQTT — third-party licenses and notices\n" + "".join(notices), encoding="utf-8")
print("Dependency notices collected in _internal/THIRD_PARTY_NOTICES.txt.")
