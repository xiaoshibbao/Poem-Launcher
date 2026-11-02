# Builds dist/Poem.exe: frontend -> Windows resources -> Go tests -> Go binary.
# Requires Go, Node.js and pnpm on PATH, plus the prebuilt DLLs listed in assets/README.md.
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

$modules = 'ProxyHook.dll', 'GameFeatures.dll', 'VisualPatch.dll', 'NetworkEntry.dll', 'LuaEntry.dll'
$missing = $modules | Where-Object { -not (Test-Path "assets/$_") }
if ($missing) { throw "Missing modules in assets/: $($missing -join ', '). See assets/README.md." }

Push-Location frontend
try {
    pnpm install --frozen-lockfile
    if ($LASTEXITCODE -ne 0) { throw 'Frontend dependency installation failed' }
    pnpm run build
    if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
} finally { Pop-Location }

Push-Location tools/winres
try {
    go run .
    if ($LASTEXITCODE -ne 0) { throw 'Windows resource generation failed' }
} finally { Pop-Location }

go test ./internal/...
if ($LASTEXITCODE -ne 0) { throw 'Tests failed' }

$version = (Get-Content build/windows/info.json -Raw | ConvertFrom-Json).info.'0000'.ProductVersion
go build -buildvcs=false -tags production -trimpath -ldflags '-w -s -H windowsgui' -o dist/Poem.exe .
if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
Copy-Item dist/Poem.exe "dist/Poem-$version.exe" -Force
Copy-Item -LiteralPath README.md, LICENSE -Destination dist
New-Item -ItemType Directory -Force dist/third-party | Out-Null
Copy-Item -LiteralPath native/lua-entry/minhook/LICENSE.txt -Destination dist/third-party/MinHook-LICENSE.txt
Write-Host "Built dist/Poem.exe (v$version)"
