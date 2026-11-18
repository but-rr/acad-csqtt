import base64
from dataclasses import asdict
import json
from pathlib import Path
import shutil
import tempfile
import time
import unittest
from unittest.mock import patch

from csqtt_client import application as app
from csqtt_client.models.profile import Profile
from csqtt_client.platform.windows import profiles
from csqtt_client.services import updates


def release(version="1.3.4"):
    filename = f"CSQTT-v{version}-Win10-x64.zip"
    return {"tag_name": f"v{version}", "assets": [{"name": filename, "state": "uploaded",
            "browser_download_url": f"https://github.com/but-rr/acad-csqtt/releases/download/v{version}/{filename}"}]}


class PortableProfileTests(unittest.TestCase):
    def test_all_fields_autosave_without_connect_and_survive_copy(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "computer-one/profile.json"
            copied = Path(directory) / "computer-two/profile.json"
            root = app.tk.Tk()
            root.withdraw()
            try:
                with patch.object(profiles, "PROFILE", source), patch.object(profiles, "LEGACY_PROFILES", ()):
                    client = app.App(root)
                    values = {name: f"saved-{name}" for name in Profile.__dataclass_fields__}
                    values.update(socks_port="1088", http_port="8088", workers="18", generation="3", obfs="video", srtp="wrap")
                    for key, value in values.items():
                        if key != "hashes":
                            client.vars[key].set(value)
                    client.hashes.insert("1.0", values["hashes"])
                    deadline = time.monotonic() + 3
                    while time.monotonic() < deadline and not source.exists():
                        root.update()
                        time.sleep(.01)
                    self.assertEqual(json.loads(source.read_text(encoding="utf-8")), values)
                    self.assertIsNone(client.core)
                    self.assertIsNone(client.helper)
                    copied.parent.mkdir()
                    shutil.copy2(source, copied)
                    with patch.object(profiles, "PROFILE", copied), patch.object(profiles, "dpapi", side_effect=AssertionError("Portable profiles must not use DPAPI")):
                        self.assertEqual(asdict(profiles.load_profile()), values)
                    client.vars["password"].set("last-keystroke")
                    client.close()
                    self.assertEqual(json.loads(source.read_text(encoding="utf-8"))["password"], "last-keystroke")
            finally:
                root.destroy()


class UpdateTests(unittest.TestCase):
    def test_version_order_and_only_official_complete_releases(self):
        self.assertIsNotNone(updates.parse_update(release("1.10.0"), "1.9.9"))
        self.assertIsNone(updates.parse_update(release("1.3.2"), "1.3.3"))
        self.assertIsNone(updates.parse_update({**release(), "draft": True}))
        self.assertIsNone(updates.parse_update({**release(), "prerelease": True}))
        data = release()
        data["assets"][0]["browser_download_url"] = "https://example.com/untrusted.zip"
        self.assertIsNone(updates.parse_update(data))

    def test_checks_use_corporate_or_local_proxy_without_direct_fallback(self):
        profile = Profile(proxy="10.0.0.1:3128", proxy_user="test-user", proxy_password="test-secret")
        with patch.object(updates.http.client, "HTTPSConnection") as factory:
            conn = factory.return_value
            conn.getresponse.return_value.status = 200
            conn.getresponse.return_value.read.return_value = json.dumps(release()).encode()
            result = updates.fetch_update(profile)
            self.assertEqual(result.version, "1.3.4")
            self.assertEqual(factory.call_args.args, ("10.0.0.1", 3128))
            token = base64.b64encode(b"test-user:test-secret").decode()
            conn.set_tunnel.assert_called_with("api.github.com", 443, headers={"Proxy-Authorization": f"Basic {token}"})
            self.assertNotIn("Proxy-Authorization", conn.request.call_args.kwargs["headers"])
            factory.reset_mock()
            updates.fetch_update(profile, local_port=8088)
            self.assertEqual(factory.call_args.args, ("127.0.0.1", 8088))
            conn.set_tunnel.assert_called_with("api.github.com", 443, headers={})
            factory.reset_mock()
            conn.request.side_effect = OSError("Proxy unavailable")
            with self.assertRaises(OSError):
                updates.fetch_update(profile)
            self.assertEqual(factory.call_count, 1)
            conn.close.assert_called()

    def test_update_banner_and_button_open_the_download(self):
        root = app.tk.Tk()
        root.withdraw()
        try:
            with patch.object(app, "load_profile", return_value=Profile()):
                client = app.App(root, persist=False)
            expected = updates.parse_update(release())
            with patch.object(client, "background", side_effect=lambda work: work()), patch.object(app, "fetch_update", return_value=expected):
                client.check_updates()
                client.poll()
            self.assertEqual(client.ui.update_banner.winfo_manager(), "pack")
            self.assertIn("1.3.4", client.ui.update_message.cget("text"))
            with patch.object(app.webbrowser, "open") as browser:
                client.download_update()
                browser.assert_called_once_with(expected.download_url)
        finally:
            root.destroy()
