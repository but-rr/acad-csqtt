"""Bundle the official signed AMD64 Wintun runtime with pinned integrity."""
import hashlib
from pathlib import Path
import urllib.request
import time
import zipfile

root = Path(__file__).resolve().parents[1]
archive = root / "build/wintun-0.14.1.zip"
expected = "07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51"
archive.parent.mkdir(exist_ok=True)
if not archive.exists() or hashlib.sha256(archive.read_bytes()).hexdigest() != expected:
    for attempt in range(4):
        try:
            with urllib.request.urlopen("https://www.wintun.net/builds/wintun-0.14.1.zip", timeout=25) as response:
                data = response.read(4 * 1024 * 1024)
            break
        except OSError:
            if attempt == 3:
                raise
            time.sleep(1)
    if hashlib.sha256(data).hexdigest() != expected:
        raise ValueError("Official Wintun archive SHA256 mismatch")
    archive.write_bytes(data)
with zipfile.ZipFile(archive) as source:
    source.extract("wintun/LICENSE.txt", root / "build/wintun")
    (root / "bin").mkdir(exist_ok=True)
    (root / "bin/wintun.dll").write_bytes(source.read("wintun/bin/amd64/wintun.dll"))
print("Verified official Wintun 0.14.1 (AMD64).")
