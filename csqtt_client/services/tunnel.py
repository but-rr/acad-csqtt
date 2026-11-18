"""Build an explicit TCP-only core command and its private environment."""
import os

from csqtt_client.config import ROOT


def core_command(profile, packet_port):
    hashes = profile.validate()
    args = [str(ROOT / "bin" / "csqtt-core.exe"), "--peer", profile.server.strip(),
            "--listen", f"127.0.0.1:{packet_port}", "--vk", ",".join(hashes),
            "--workers", profile.workers, "--exact-workers", "--turn-transport", "tcp", "--port", "443",
            "--obfs", profile.obfs, "--srtp-mode", profile.srtp, "--gen", profile.generation, "--salt", profile.salt]
    if profile.turn.strip():
        args += ["--turn", profile.turn.strip()]
    env = os.environ.copy()
    # Ignore inherited bypasses and proxy credentials. The core uses this explicit proxy.
    for key in list(env):
        if key.upper() in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "CSQTT_HTTP_PROXY", "CSQTT_PROXY_USER", "CSQTT_PROXY_PASSWORD"):
            del env[key]
    env.update(CSQTT_PASSWORD=profile.password, CSQTT_EVENTS="1")
    if profile.internet_mode == "proxy":
        env.update(CSQTT_HTTP_PROXY=profile.proxy.strip(), CSQTT_PROXY_USER=profile.proxy_user,
                   CSQTT_PROXY_PASSWORD=profile.proxy_password)
    return args, env
