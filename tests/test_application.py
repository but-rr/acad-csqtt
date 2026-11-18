import base64
from dataclasses import asdict, replace
import json
import os
import socket
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from unittest.mock import patch

from csqtt_client import application as app
from csqtt_client.models.profile import normalize_hashes
from csqtt_client.platform.windows import profiles


class ClientTests(unittest.TestCase):
    def profile(self):
        return app.Profile(server="203.0.113.2:443", password="server-test-secret",
                           hashes="testhash123456", proxy_user="proxy-test-user", proxy_password="proxy-test-secret")

    def test_tcp_proxy_command_and_secret_environment(self):
        profile = self.profile()
        args, env = app.core_command(profile, 9000)
        self.assertEqual(args[args.index("--turn-transport") + 1], "tcp")
        self.assertEqual(args[args.index("--port") + 1], "443")
        self.assertNotIn(profile.password, args)
        self.assertNotIn(profile.proxy_password, args)
        self.assertEqual(env["CSQTT_PASSWORD"], profile.password)
        self.assertEqual(env["CSQTT_PROXY_PASSWORD"], profile.proxy_password)
        self.assertNotIn("NO_PROXY", env)

    def test_hash_normalization_and_validation(self):
        self.assertEqual(normalize_hashes("https://vk.com/call/join/abc123?x=1\nabc123;def456"), ["abc123", "def456"])
        for profile in [replace(self.profile(), hashes=""), replace(self.profile(), socks_port="8080"),
                        replace(self.profile(), workers="0"), replace(self.profile(), proxy="proxy.invalid:3128"),
                        replace(self.profile(), turn="turn:example.com:443"), replace(self.profile(), password="")]:
            with self.assertRaises(ValueError):
                profile.validate()

    @unittest.skipUnless(os.name == "nt", "Windows DPAPI")
    def test_profile_encryption_roundtrip(self):
        plain = json.dumps({"secret": "test-only-secret"}).encode()
        encrypted = profiles.dpapi(plain)
        self.assertNotIn(b"test-only-secret", encrypted)
        self.assertEqual(profiles.dpapi(encrypted, decrypt=True), plain)

    @unittest.skipUnless(os.name == "nt", "Windows DPAPI")
    def test_legacy_profile_load_save_and_forget(self):
        with tempfile.TemporaryDirectory() as directory:
            legacy = app.Path(directory) / "old/profile.dat"
            current = app.Path(directory) / "new/profile.json"
            legacy.parent.mkdir()
            legacy.write_bytes(profiles.dpapi(json.dumps(asdict(self.profile())).encode()))
            with patch.object(profiles, "PROFILE", current), patch.object(profiles, "LEGACY_PROFILES", (legacy,)):
                self.assertEqual(profiles.load_profile(), self.profile())
                self.assertTrue(profiles.profile_exists())
                profiles.save_profile(profiles.load_profile())
                self.assertTrue(current.exists())
                self.assertIn(self.profile().password, current.read_text(encoding="utf-8"))
                self.assertEqual(profiles.load_profile(), self.profile())
                profiles.delete_profile()
                self.assertFalse(profiles.profile_exists())

    def fake_proxy(self, status):
        listener = socket.socket()
        listener.bind(("127.0.0.1", 0))
        listener.listen()
        captured = []
        def handle():
            try:
                with listener.accept()[0] as conn:
                    data = bytearray()
                    while not data.endswith(b"\r\n\r\n"):
                        data.extend(conn.recv(1))
                    captured.append(bytes(data))
                    conn.sendall(f"HTTP/1.1 {status}\r\n\r\n".encode())
            finally:
                listener.close()
        thread = threading.Thread(target=handle)
        thread.start()
        return f"127.0.0.1:{listener.getsockname()[1]}", captured, thread

    def test_authenticated_connect(self):
        address, captured, thread = self.fake_proxy("200 Connection established")
        profile = replace(self.profile(), proxy=address)
        app.proxy_connect(profile, "turn.example:443")
        thread.join(timeout=3)
        expected = base64.b64encode(f"{profile.proxy_user}:{profile.proxy_password}".encode())
        self.assertIn(b"CONNECT turn.example:443 HTTP/1.1", captured[0])
        self.assertIn(b"Proxy-Authorization: Basic " + expected, captured[0])

    def test_proxy_auth_failure(self):
        address, captured, thread = self.fake_proxy("407 Proxy Authentication Required")
        with self.assertRaisesRegex(ValueError, "407"):
            app.proxy_connect(replace(self.profile(), proxy=address), "turn.example:443")
        thread.join(timeout=3)

    @unittest.skipUnless((app.ROOT / "bin/csqtt-core.exe").exists(), "Build the Windows core first")
    def test_built_core_routes_vk_through_authenticated_proxy(self):
        address, captured, thread = self.fake_proxy("407 Proxy Authentication Required")
        profile = replace(self.profile(), proxy=address)
        args, env = app.core_command(profile, 19090)
        process = subprocess.Popen(args, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                   stderr=subprocess.STDOUT, creationflags=app.NO_WINDOW)
        try:
            thread.join(timeout=12)
            self.assertTrue(captured, "Built core did not contact the explicit proxy")
            self.assertIn(b"CONNECT api.vk.me:443 HTTP/1.1", captured[0])
            expected = base64.b64encode(f"{profile.proxy_user}:{profile.proxy_password}".encode())
            self.assertIn(b"proxy-authorization: basic " + expected.lower(), captured[0].lower())
            output, _ = process.communicate(b"STOP\n", timeout=8)
            self.assertNotIn(profile.password.encode(), output)
            self.assertNotIn(profile.proxy_password.encode(), output)
        finally:
            if process.poll() is None:
                process.kill()
                process.communicate()


@unittest.skipUnless(os.name == "nt", "Windows native GUI")
class LogExportTests(unittest.TestCase):
    def setUp(self):
        self.root = app.tk.Tk()
        self.root.withdraw()
        with patch.object(app, "load_profile", return_value=app.Profile()):
            self.ui = app.App(self.root, persist=False)
        self.ui.secrets = ["test-only-password", "test-only-hash"]
        self.ui.append("Ошибка потока 18: test-only-password test-only-hash")
        self.expected = self.ui.log.get("1.0", "end-1c")

    def tearDown(self):
        self.root.destroy()

    def test_copy_disabled_log_and_selection(self):
        self.ui.copy_log()
        self.assertEqual(self.root.clipboard_get(), self.expected)
        self.assertNotIn("test-only-password", self.root.clipboard_get())
        self.assertNotIn("test-only-hash", self.root.clipboard_get())
        self.assertIn("[скрыто]", self.root.clipboard_get())
        self.ui.log.tag_add("sel", "1.0", "1.5")
        selected = self.ui.log.get("sel.first", "sel.last")
        self.ui.copy_log(selected=True)
        self.assertEqual(self.root.clipboard_get(), selected)
        self.ui.select_log()
        self.assertEqual(self.ui.log.get("sel.first", "sel.last"), self.expected)
        self.assertEqual(self.ui.log.cget("state"), "disabled")

    def test_save_masked_utf8_log(self):
        with tempfile.TemporaryDirectory() as directory:
            filename = app.Path(directory) / "журнал.txt"
            with patch.object(app.filedialog, "asksaveasfilename", return_value=str(filename)):
                self.ui.save_log()
            self.assertEqual(filename.read_text(encoding="utf-8"), self.expected)

    def test_pages_keep_profile_and_show_live_statistics(self):
        self.ui.vars["server"].set("203.0.113.2:46000")
        self.ui.vars["password"].set("test-only-password")
        self.ui.hashes.insert("1.0", "test-only-hash")
        self.ui.ui.secret_buttons["password"].invoke()
        self.assertEqual(self.ui.ui.entries["password"].cget("show"), "")
        self.ui.ui.secret_buttons["password"].invoke()
        self.assertNotEqual(self.ui.ui.entries["password"].cget("show"), "")
        self.ui.ui.show_page("settings")
        self.ui.vars["http_port"].set("8181")
        self.ui.ui.show_page("logs")
        self.ui.process_line("core", '__CSQTT_EVENT__|STATS|{"active":12,"bytes_up":1048576,"bytes_down":2097152}')
        self.assertEqual(self.ui.active_count.get(), "12")
        self.assertEqual(self.ui.upload_total.get(), "1.00")
        self.assertEqual(self.ui.download_total.get(), "2.00")
        self.ui.ui.show_page("connection")
        self.assertEqual(self.ui.profile().server, "203.0.113.2:46000")
        self.assertEqual(self.ui.profile().password, "test-only-password")
        self.assertEqual(self.ui.profile().hashes, "test-only-hash")
        self.ui.ui.copy_endpoint("http_port", "HTTP")
        self.assertEqual(self.root.clipboard_get(), "http://127.0.0.1:8181")

    def test_proxy_check_result_is_shown_on_connection_page(self):
        with patch.object(self.ui, "background", side_effect=lambda work: work()):
            with patch.object(app, "proxy_connect"):
                self.ui.check_proxy()
                self.ui.poll()
            self.assertEqual(self.ui.ui.page, "connection")
            self.assertIn("доступен", self.ui.ui.proxy_feedback.cget("text"))
            self.assertEqual(self.ui.ui.proxy_feedback.cget("fg"), "#79c9a4")
            with patch.object(app, "proxy_connect", side_effect=ValueError("Ошибка авторизации 407")):
                self.ui.check_proxy()
                self.ui.poll()
            self.assertEqual(self.ui.ui.page, "connection")
            self.assertIn("407", self.ui.ui.proxy_feedback.cget("text"))
            self.assertEqual(self.ui.ui.proxy_feedback.cget("fg"), "#df9499")
            self.assertFalse(self.ui.check_proxy_button.instate(["disabled"]))

    def test_dragging_and_switching_pages_keep_window_responsive(self):
        self.root.deiconify()
        self.root.update()
        desktop = self.ui.ui
        pages = [getattr(desktop, name) for name in ("connection", "settings", "logs")]
        unmapped = []
        for page in pages:
            page.bind("<Unmap>", lambda event: unmapped.append(event.widget))
        self.ui.vars["server"].set("203.0.113.2:443")
        self.ui.hashes.insert("1.0", "test-hash-for-page-switch")
        for _ in range(10):
            for name in ("settings", "logs", "connection"):
                desktop.show_page(name)
                self.root.update()
                self.assertTrue(all(page.winfo_ismapped() for page in pages))
        self.assertFalse(unmapped, "Page switches must not unmap and rebuild the UI")
        self.assertEqual(self.ui.profile().hashes, "test-hash-for-page-switch")
        focused = str(self.ui.ui.entries["server"])
        for _ in range(30):
            focused = str(self.root.tk.call("tk_focusNext", focused))
            self.assertFalse(focused.startswith(str(desktop.settings)))
            self.assertFalse(focused.startswith(str(desktop.logs)))
        bar = desktop.titlebar
        x, y = self.root.winfo_x(), self.root.winfo_y()
        size = (self.root.winfo_width(), self.root.winfo_height())
        bar.event_generate("<ButtonPress-1>", x=250, y=20, rootx=x+250, rooty=y+20)
        heartbeats = []
        for delta in range(5, 101, 5):
            self.root.after_idle(lambda: heartbeats.append(1))
            bar.event_generate("<B1-Motion>", x=250, y=20, rootx=x+250+delta, rooty=y+20+delta)
            self.root.update()
        bar.event_generate("<ButtonRelease-1>", x=250, y=20)
        self.root.update()
        self.assertEqual(len(heartbeats), 20)
        self.assertEqual((self.root.winfo_x(), self.root.winfo_y()), (x+100, y+100))
        self.assertEqual((self.root.winfo_width(), self.root.winfo_height()), size)
        desktop.maximize_button.invoke()
        self.root.update()
        self.assertEqual(self.root.state(), "zoomed")
        bar.event_generate("<ButtonPress-1>", x=250, y=20, rootx=250, rooty=20)
        bar.event_generate("<B1-Motion>", x=250, y=20, rootx=300, rooty=80)
        bar.event_generate("<ButtonRelease-1>", x=250, y=20)
        self.root.update()
        self.assertEqual(self.root.state(), "normal")


@unittest.skipUnless(os.name == "nt", "Windows tray")
class TrayTests(unittest.TestCase):
    def test_hide_restore_and_full_exit_during_disconnect(self):
        root = app.tk.Tk()
        root.withdraw()
        processes = []
        tray = None

        def pump_until(condition, timeout=7):
            deadline = time.monotonic() + timeout
            while time.monotonic() < deadline:
                root.update()
                if condition():
                    return
                time.sleep(0.01)
            self.fail("GUI did not reach the expected state")

        try:
            with patch.object(app, "load_profile", return_value=app.Profile()):
                ui = app.App(root, persist=False)
            ui.start_tray()
            tray = ui.tray
            pump_until(tray.ready.is_set)
            self.assertTrue(tray.icon.visible)
            core = subprocess.Popen([sys.executable, "-c", "import sys; sys.stdin.readline()"],
                                    stdin=subprocess.PIPE, stdout=subprocess.PIPE, text=True,
                                    creationflags=app.NO_WINDOW)
            processes.append(core)
            helper = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(60)"],
                                      stdout=subprocess.PIPE, text=True, creationflags=app.NO_WINDOW)
            processes.append(helper)
            ui.core, ui.helper = core, helper
            ui.hide_to_tray()
            self.assertEqual(root.state(), "withdrawn")
            # Invoke the real menu callback from another thread, as pystray does.
            opened = threading.Thread(target=lambda: tray.icon.menu.items[0](tray.icon))
            opened.start()
            opened.join()
            pump_until(lambda: root.state() == "normal")
            self.assertTrue(ui.ui.chrome.hwnd)
            ui.ui.maximize_button.invoke()
            pump_until(lambda: root.state() == "zoomed")
            ui.ui.maximize_button.invoke()
            pump_until(lambda: root.state() == "normal")
            ui.ui.minimize_button.invoke()
            pump_until(lambda: root.state() == "withdrawn")
            ui.show_window()
            pump_until(lambda: root.state() == "normal")
            root.iconify()
            pump_until(lambda: root.state() == "withdrawn")
            ui.show_window()
            root.update()
            root.tk.call(root.protocol("WM_DELETE_WINDOW"))
            self.assertEqual(root.state(), "withdrawn")
            ui.show_window()
            pump_until(lambda: root.state() == "normal")
            ui.ui.hide_button.invoke()
            self.assertEqual(root.state(), "withdrawn")
            self.assertTrue(all(p.poll() is None for p in processes))
            ui.stop()
            exiting = threading.Thread(target=lambda: tray.icon.menu.items[-1](tray.icon))
            exiting.start()
            exiting.join()
            pump_until(lambda: ui.tray is None)
            self.assertTrue(ui.closing)
            self.assertFalse(tray.thread.is_alive())
            self.assertTrue(all(p.poll() is not None for p in processes))
        finally:
            if tray:
                tray.stop()
            for process in processes:
                if process.poll() is None:
                    process.kill()
                    process.wait()
            try:
                root.destroy()
            except app.tk.TclError:
                pass


if __name__ == "__main__":
    unittest.main()
