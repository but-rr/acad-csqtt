# CSQTT Windows launcher

This distribution contains a modified CSQTT client, based on the user's supplied
`csqtt-2.5.0.7z` archive and the amurcanov/csqtt project.
Upstream: https://github.com/amurcanov/csqtt
Copyright 2026 amurcanov and upstream contributors.
CSQTT source is subject to PolyForm Noncommercial License 1.0.0 in LICENSE.
The upstream Cargo manifest's Unlicense identifier does not replace the supplied
repository license. Preserve the repository license when redistributing.

Modifications: explicit authenticated HTTP CONNECT for TURN/TCP/TLS and VK API,
proxy-only bootstrap DNS over HTTPS, rejection of TURN/UDP in proxy mode,
worker-count renegotiation on established streams after reconnect,
native Windows launcher, DPAPI profile storage, and local SOCKS5/HTTP proxies.

The packet proxy uses the WireGuard Go netstack package (MIT) and gVisor
(Apache 2.0); it does not establish a WireGuard connection.
The GUI is packaged with Python, Tcl/Tk and PyInstaller.
Third-party copyright and license texts are provided in _internal/THIRD_PARTY_NOTICES.txt.

The UI includes Nunito Sans, Copyright 2016 The Nunito Sans Project Authors,
under the SIL Open Font License 1.1, provided in assets/fonts/OFL.txt.

No proxy passwords, CSQTT server credentials or personal profiles are included
in the release archive. The application creates profile.json beside its EXE
when saving is enabled. This portable file contains unencrypted credentials.
