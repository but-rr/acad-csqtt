"""Read public release metadata over an explicit HTTP CONNECT proxy."""
import base64
from dataclasses import dataclass
import http.client
import ipaddress
import json
import re
import ssl

from csqtt_client.config import APP_VERSION
from csqtt_client.models.profile import split_address

REPOSITORY = "but-rr/acad-csqtt"
API_PATH = f"/repos/{REPOSITORY}/releases/latest"
MAX_RESPONSE = 256 * 1024


@dataclass(frozen=True)
class Update:
    version: str
    download_url: str


def version_tuple(value):
    match = re.fullmatch(r"v?(\d+)\.(\d+)\.(\d+)", value)
    if not match:
        raise ValueError("Unsupported release version")
    return tuple(int(part) for part in match.groups())


def parse_update(data, current_version=APP_VERSION):
    if data.get("draft") or data.get("prerelease"):
        return None
    tag = data.get("tag_name", "")
    if not tag.startswith("v") or version_tuple(tag) <= version_tuple(current_version):
        return None
    filename = f"CSQTT-{tag}-Win10-x64.zip"
    expected_url = f"https://github.com/{REPOSITORY}/releases/download/{tag}/{filename}"
    for asset in data.get("assets", []):
        if asset.get("name") == filename and asset.get("browser_download_url") == expected_url and asset.get("state") == "uploaded":
            return Update(tag[1:], expected_url)
    return None


def fetch_update(profile, *, local_port=None):
    tunnel_headers = {}
    if local_port is not None:
        host, port = "127.0.0.1", local_port
    else:
        host, port = split_address(profile.proxy)
        ipaddress.ip_address(host)
        if profile.proxy_user:
            token = base64.b64encode(f"{profile.proxy_user}:{profile.proxy_password}".encode()).decode()
            tunnel_headers["Proxy-Authorization"] = f"Basic {token}"
    connection = http.client.HTTPSConnection(host, port, timeout=8, context=ssl.create_default_context())
    try:
        connection.set_tunnel("api.github.com", 443, headers=tunnel_headers)
        connection.request("GET", API_PATH, headers={
            "Accept": "application/vnd.github+json",
            "User-Agent": f"CSQTT/{APP_VERSION}",
            "X-GitHub-Api-Version": "2022-11-28",
        })
        response = connection.getresponse()
        if response.status != 200:
            raise OSError(f"GitHub HTTP {response.status}")
        body = response.read(MAX_RESPONSE + 1)
        if len(body) > MAX_RESPONSE:
            raise ValueError("Release metadata is too large")
        return parse_update(json.loads(body))
    finally:
        connection.close()
