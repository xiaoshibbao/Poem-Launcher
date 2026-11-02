# Process-local network entry

UnityPlayer imports WinHttpGetIEProxyConfigForCurrentUser and Winsock connect.
The 2026-10-21 traces showed dispatch and SDK requests entering the local system
HTTP proxy even with per-process HTTP_PROXY set. The launcher forwarding itself
was already reaching the private server; interface binding did not fix ingress.

This native module substitutes the IE-proxy query result in the game process,
returning Poem's actual loopback listening port. It also reroutes connections to
the configured local HTTP system-proxy port to Poem, covering cached clients.
Only process import tables are changed; system registry, system proxy and Clash
configuration are unchanged. ProxyHook/Poem.Proxy modules are excluded to avoid
changing the network export addresses used by their existing hook installer.
New DLL imports are patched after LoadLibrary returns, and a native worker
covers modules loaded via other paths. The DLL remains pinned for callback
lifetime. Thread returns use normal OS cleanup, not forced thread termination.

The injector waits for the initial import-table scan before resuming main.
All native code and tests are here. Rebuild with scripts/build-network.ps1 using
an officially checksummed Zig C toolchain. The prebuilt 22 KiB DLL is embedded.
No C runtime/toolchain installation is needed on user computers.

Verified in disposable processes: process-local WinHTTP proxy query, imported
and dynamically resolved connect/WSAConnect, successful forwarding to a fake
private server with zero requests reaching a fake system proxy, unrelated port
preservation, and initialization before the target main function resumes.
These checks do not establish actual game-wide compatibility. Separate browser
processes are not injected, SOCKS-only/PAC and TUN-only configurations are outside
the local HTTP-system-proxy case covered here.
