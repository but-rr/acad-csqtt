"""Connection mode boundaries, controller commands, and the startup screen."""
from dataclasses import replace
import os
import unittest
from unittest.mock import Mock, patch

from csqtt_client import application as app
from csqtt_client.services import updates


class ConnectionModeTests(unittest.TestCase):
    def profile(self, **values):
        return replace(app.Profile(server="203.0.113.2:443", password="test-secret", hashes="test-hash"), **values)

    def test_direct_ignores_corporate_fields_and_inherited_credentials(self):
        profile = self.profile(internet_mode="direct", proxy="", proxy_user="old", proxy_password="")
        with patch.dict(os.environ, {"CSQTT_HTTP_PROXY": "old", "CSQTT_PROXY_USER": "old", "CSQTT_PROXY_PASSWORD": "secret", "HTTPS_PROXY": "old"}):
            args, env = app.core_command(profile, 9000)
        self.assertEqual(args[args.index("--turn-transport") + 1], "tcp")
        for key in ("CSQTT_HTTP_PROXY", "CSQTT_PROXY_USER", "CSQTT_PROXY_PASSWORD", "HTTPS_PROXY"):
            self.assertNotIn(key, env)
        with self.assertRaises(ValueError):
            self.profile(proxy="").validate()

    def test_update_check_uses_direct_https_only_when_selected(self):
        with patch.object(updates.http.client, "HTTPSConnection") as factory:
            factory.return_value.getresponse.return_value.status = 503
            with self.assertRaises(OSError):
                updates.fetch_update(self.profile(internet_mode="direct", proxy=""))
            self.assertEqual(factory.call_args.args, ("api.github.com", 443))
            factory.return_value.set_tunnel.assert_not_called()

    def test_tun_launch_uses_real_parent_and_routes_all_known_turn_ips(self):
        root = app.tk.Tk()
        root.withdraw()
        try:
            with patch.object(app, "load_profile", return_value=self.profile(tun="1", internet_mode="direct", system_proxy="1")):
                client = app.App(root, persist=False)
            self.assertEqual(client.ui.page, "connection")
            self.assertTrue(str(client.ui.entries["server"]).startswith(str(client.ui.settings)))
            client.running_profile = client.profile()
            client.ready, client.config, client.packet_port = True, ("10.23.0.2", "1.1.1.1"), 19090
            with patch.object(app.subprocess, "Popen") as launch, patch.object(client, "read_process"):
                client.start_helper()
                launch.assert_not_called()  # No bypass route: never install a recursive default route.
                client.tun_bypass_ips = ["203.0.113.10", "203.0.113.11"]
                client.start_helper()
                args = launch.call_args.args[0]
                self.assertEqual(args[args.index("--bypass-ip") + 1], "203.0.113.10,203.0.113.11")
                self.assertEqual(args[args.index("--parent-pid") + 1], str(os.getpid()))
                self.assertIn("--system-proxy=true", args)
                self.assertIsNotNone(launch.call_args.kwargs["stdin"])
            client.helper = Mock()
            client.vars["system_proxy"].set("0")
            client.system_proxy_changed()
            client.helper.stdin.write.assert_called_with("SYSTEM_PROXY_OFF\n")
            client.process_line("core", "TUN_BYPASS:203.0.113.12")
            client.helper.stdin.write.assert_called_with("BYPASS 203.0.113.12\n")
            client.ui.set_state("connecting")
            self.assertTrue(client.ui.corporate_switch.instate(["disabled"]))
            client.ui.set_state("offline")
            self.assertFalse(client.ui.corporate_switch.instate(["disabled"]))
        finally:
            root.destroy()
