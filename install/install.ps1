# Copyright (c) 2026 Derick Schaefer
# Licensed under the MIT License. See LICENSE file for details.

param(
    [string]$Version = "latest",
    [string]$BaseUrl = "https://download.reservecli.dev"
)

$ErrorActionPreference = "Stop"

function Fail {
    param([string]$Message)
    throw "reserve install: $Message"
}

function Get-Arch {
    $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    switch ($arch.ToUpperInvariant()) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { Fail "unsupported architecture: $arch" }
    }
}

$arch = Get-Arch
$archive = "reserve_windows_${arch}.zip"
$url = "$BaseUrl/releases/$Version/$archive"
$tmpDir = Join-Path ([System.IO.Path]::GetTempPath()) ("reserve-install-" + [System.Guid]::NewGuid().ToString("N"))
$zipPath = Join-Path $tmpDir $archive
$extractDir = Join-Path $tmpDir "extract"
$installDir = Join-Path $env:USERPROFILE "bin"
$target = Join-Path $installDir "reserve.exe"

New-Item -ItemType Directory -Force -Path $tmpDir | Out-Null
New-Item -ItemType Directory -Force -Path $extractDir | Out-Null
New-Item -ItemType Directory -Force -Path $installDir | Out-Null

try {
    Write-Host "reserve install: downloading $url"
    Invoke-RestMethod -Uri $url -OutFile $zipPath

    Write-Host "reserve install: extracting $archive"
    Expand-Archive -Path $zipPath -DestinationPath $extractDir -Force

    $binary = Join-Path $extractDir "reserve.exe"
    if (-not (Test-Path $binary)) {
        Fail "archive did not contain expected binary: reserve.exe"
    }

    Move-Item -Force $binary $target
    Write-Host ""
    Write-Host "reserve install: installed to $target"

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $pathEntries = @()
    if ($userPath) {
        $pathEntries = $userPath.Split(';', [System.StringSplitOptions]::RemoveEmptyEntries)
    }

    if ($pathEntries -notcontains $installDir) {
        $newPath = if ($userPath) { "$userPath;$installDir" } else { $installDir }
        [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
        Write-Host "reserve install: added $installDir to your user PATH"
        Write-Host "reserve install: restart your terminal to pick up the PATH change"
    }

    Write-Host "reserve install: run 'reserve version' to verify the installation"
}
finally {
    if (Test-Path $tmpDir) {
        Remove-Item -Recurse -Force $tmpDir
    }
}
