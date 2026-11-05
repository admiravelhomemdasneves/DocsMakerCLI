# Installs docsmaker for the current user (no administrator rights needed).
# Run from the extracted release folder:  .\install.ps1
#
# It copies docsmaker.exe to %LOCALAPPDATA%\Programs\docsmaker and adds that
# folder to your user PATH. Nothing else is changed and nothing is downloaded.
$ErrorActionPreference = "Stop"

$source = Join-Path $PSScriptRoot "docsmaker.exe"
if (-not (Test-Path $source)) { throw "docsmaker.exe not found next to this script." }

$dest = Join-Path $env:LOCALAPPDATA "Programs\docsmaker"
New-Item -ItemType Directory -Force $dest | Out-Null
Copy-Item $source $dest -Force
Write-Host "installed $dest\docsmaker.exe"

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$entries = @()
if ($userPath) { $entries = $userPath -split ";" }
if ($entries -notcontains $dest) {
    [Environment]::SetEnvironmentVariable("Path", (($entries + $dest) -join ";").Trim(";"), "User")
    Write-Host "added $dest to your user PATH (open a new terminal to use it)"
}

Write-Host ""
Write-Host "Next steps:"
Write-Host "  docsmaker model pull      # installs/starts Ollama and downloads the model (asks first)"
Write-Host "  docsmaker --help"
