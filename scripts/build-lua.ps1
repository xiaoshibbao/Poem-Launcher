# Builds assets/LuaEntry.dll (local Lua bridge) from native/lua-entry with Zig.
param([string]$Zig = $(if ($env:ZIG) { $env:ZIG } else { 'zig' }))
$ErrorActionPreference = 'Stop'
Push-Location (Split-Path $PSScriptRoot -Parent)
try {
    & $Zig cc -O2 -s -target x86_64-windows-gnu -shared native/lua-entry/lua_entry.c native/lua-entry/minhook/src/buffer.c native/lua-entry/minhook/src/hook.c native/lua-entry/minhook/src/trampoline.c native/lua-entry/minhook/src/hde/hde64.c -o assets/LuaEntry.dll -luser32
    if ($LASTEXITCODE -ne 0) { throw 'Lua bridge build failed' }
} finally { Pop-Location }
