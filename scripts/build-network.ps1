# Builds assets/NetworkEntry.dll (native network bridge) and its test fixture with Zig.
param([string]$Zig = $(if ($env:ZIG) { $env:ZIG } else { 'zig' }))
$ErrorActionPreference = 'Stop'
Push-Location (Split-Path $PSScriptRoot -Parent)
try {
    & $Zig cc -shared -O2 -s -Wall -Wextra -target x86_64-windows-gnu native/network-entry/network_entry.c -o assets/NetworkEntry.dll -lws2_32 -lwinhttp
    if ($LASTEXITCODE -ne 0) { throw 'Native network-entry build failed' }
    New-Item -ItemType Directory -Force qa | Out-Null
    & $Zig cc -O2 -s -target x86_64-windows-gnu native/network-entry/fixture.c -o qa/Poem-network-fixture.exe -lws2_32 -lwinhttp
    if ($LASTEXITCODE -ne 0) { throw 'Native network fixture build failed' }
} finally { Pop-Location }
