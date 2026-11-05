# Builds docsmaker for Windows and packages it into dist\.
#
# The tree-sitter grammar is C code, so the build needs CGO and gcc on PATH
# (e.g. WinLibs: winget install BrechtSanders.WinLibs.POSIX.UCRT). Install the
# toolchain in a path WITHOUT spaces, e.g. C:\mingw64: the MinGW linker breaks
# on spaces. Not cross-compilable: run it on Windows.
#
#   $env:VERSION = "v1.0.0"; scripts\build.ps1
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
    throw "gcc not found on PATH. Install a MinGW-w64 toolchain (see the header of this script)."
}

$version = $env:VERSION
if (-not $version) {
    $version = (git describe --tags --always --dirty 2>$null)
    if (-not $version) { $version = "dev" }
}
$arch = (go env GOARCH)
$name = "docsmaker_${version}_windows_${arch}"
$out  = "dist\$name"
$zip  = "dist\$name.zip"

Remove-Item -Recurse -Force $out, $zip -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $out | Out-Null

Write-Host "building $name ..."
$env:CGO_ENABLED = "1"
go build -trimpath -tags "netgo,osusergo" `
    -ldflags "-s -w -X github.com/admiravelhomemdasneves/DocsMakerCLI/internal/cli.Version=$version" `
    -o "$out\docsmaker.exe" ./cmd/docsmaker
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

Copy-Item README.md, scripts\install.ps1 $out

# Smoke test the artifact before shipping it.
& "$out\docsmaker.exe" --version
if ($LASTEXITCODE -ne 0) { throw "the built binary does not run" }

Compress-Archive -Path $out -DestinationPath $zip
$hash = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
"$hash  $name.zip" | Out-File -Encoding ascii "$zip.sha256"
Write-Host "wrote $zip"
