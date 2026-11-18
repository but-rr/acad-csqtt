$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path -Parent $PSScriptRoot
Set-Location -LiteralPath $projectRoot
$version = (Get-Content -LiteralPath VERSION -Raw).Trim()
if ($version -notmatch '^\d+\.\d+\.\d+$') { throw 'Invalid VERSION' }
if ($env:GITHUB_REF_TYPE -eq 'tag' -and $env:GITHUB_REF_NAME -ne "v$version") { throw 'Tag must match VERSION' }
if ((Get-Content -LiteralPath wails.json -Raw | ConvertFrom-Json).info.productVersion -ne $version) { throw 'Version mismatch' }
if (Test-Path '.tools/cargo/bin/cargo.exe') {
 $env:CARGO_HOME = Join-Path $projectRoot '.tools/cargo'
 $env:RUSTUP_HOME = Join-Path $projectRoot '.tools/rustup'
 $env:PATH = "$env:CARGO_HOME\bin;$env:PATH"
}
& cargo build --release --locked --manifest-path core/rust-client/Cargo.toml
if ($LASTEXITCODE -ne 0) { throw 'Core build failed' }
New-Item -ItemType Directory -Force bin,build/windows,.tools/bin | Out-Null
& go run scripts/package.go wintun
if ($LASTEXITCODE -ne 0) { throw 'Wintun preparation failed' }
Copy-Item -LiteralPath core/rust-client/target/release/client.exe -Destination bin/csqtt-core.exe -Force
& go -C packet-proxy build -trimpath -ldflags '-s -w' -o ../bin/packet-proxy.exe .
if ($LASTEXITCODE -ne 0) { throw 'Packet helper build failed' }
Copy-Item -LiteralPath assets/icons/CS_logo.png -Destination build/appicon.png -Force
Copy-Item -LiteralPath assets/icons/CSQTT.ico -Destination build/windows/icon.ico -Force
$wails = Join-Path $projectRoot '.tools/bin/wails.exe'
if (-not (Test-Path -LiteralPath $wails)) {
 $previousGoBin = $env:GOBIN
 try { $env:GOBIN = Split-Path -Parent $wails; & go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0; if ($LASTEXITCODE -ne 0) { throw 'Wails install failed' } }
 finally { $env:GOBIN = $previousGoBin }
}
Push-Location frontend
try {
 if (-not (Test-Path node_modules)) { & npm.cmd ci --no-audit --no-fund; if ($LASTEXITCODE -ne 0) { throw 'Frontend install failed' } }
 & npm.cmd run build
 if ($LASTEXITCODE -ne 0) { throw 'Frontend build failed' }
} finally { Pop-Location }
& $wails build -s -skipbindings -trimpath -platform windows/amd64 -webview2 embed
if ($LASTEXITCODE -ne 0) { throw 'Wails build failed' }
$packageDir = Join-Path $projectRoot 'dist/CSQTT'
if (Test-Path -LiteralPath $packageDir) {
 $resolvedPackage = (Resolve-Path -LiteralPath $packageDir).Path
 if (-not $resolvedPackage.StartsWith($projectRoot + '\', [StringComparison]::OrdinalIgnoreCase)) { throw 'Package must stay inside workspace' }
 Remove-Item -LiteralPath $resolvedPackage -Recurse -Force
}
New-Item -ItemType Directory -Force "$packageDir/bin","$packageDir/_internal" | Out-Null
Copy-Item -LiteralPath build/bin/CSQTT.exe -Destination $packageDir -Force
foreach ($file in @('csqtt-core.exe','packet-proxy.exe','wintun.dll')) { Copy-Item -LiteralPath "bin/$file" -Destination "$packageDir/bin" -Force }
Copy-Item -LiteralPath "$env:SystemRoot/System32/vcruntime140.dll" -Destination "$packageDir/bin/VCRUNTIME140.dll" -Force
Copy-Item -LiteralPath VERSION -Destination "$packageDir/_internal" -Force
& go run scripts/package.go notices $packageDir
if ($LASTEXITCODE -ne 0) { throw 'Notices collection failed' }
$smoke = Start-Process -FilePath "$packageDir/CSQTT.exe" -ArgumentList '--smoke' -WindowStyle Hidden -PassThru
if (-not $smoke.WaitForExit(15000)) { $smoke.Kill(); throw 'GUI startup timeout' }
if ($smoke.ExitCode -ne 0) { throw 'GUI startup failed' }
& go run scripts/package.go zip $packageDir "CSQTT-v$version-Win10-x64.zip"
if ($LASTEXITCODE -ne 0) { throw 'ZIP packaging failed' }
