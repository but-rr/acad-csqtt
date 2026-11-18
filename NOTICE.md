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
native Windows launcher, portable profile storage, local SOCKS5/HTTP proxies,
Windows system proxy management, direct internet mode and native Wintun routing.
The initial Windows TUN implementation was adapted from ARES-glitch-cpu/acad-csqtt
(pull request 1, revision 04f7191e8bd407857c16092d36aeb9e8ab9cd71f).

The packet proxy uses the WireGuard Go netstack package (MIT) and gVisor
(Apache 2.0); it does not establish a WireGuard connection.
The desktop shell uses Wails v2 (MIT), Svelte (MIT), TypeScript and WebView2.
The prior Python/Tkinter shell is no longer distributed.
The Windows distribution includes the official signed Wintun 0.14.1 AMD64 DLL.
Its binary distribution license is included in the third-party notices below.
Third-party copyright and license texts are provided in _internal/THIRD_PARTY_NOTICES.txt.

The UI includes Nunito Sans, Copyright 2016 The Nunito Sans Project Authors,
under the SIL Open Font License 1.1, provided in assets/fonts/OFL.txt.

No proxy passwords, CSQTT server credentials or personal profiles are included
in the release archive. The application creates profile.json beside its EXE
when saving is enabled. This portable file contains unencrypted credentials.
