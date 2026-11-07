param(
  [Parameter(Mandatory = $true)]
  [string]$DistDir
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$dist = (Resolve-Path -LiteralPath $DistDir).Path
$metadataPath = Join-Path $dist 'metadata.json'
$metadata = Get-Content -Raw -LiteralPath $metadataPath | ConvertFrom-Json
$version = [string]$metadata.version

if ([string]::IsNullOrWhiteSpace($version)) {
  throw 'Release metadata has no version'
}

if ($IsWindows) {
  $platform = 'windows'
  $extension = '.zip'
  $executable = 'build-bouncer.exe'
} elseif ($IsMacOS) {
  $platform = 'darwin'
  $extension = '.tar.gz'
  $executable = 'build-bouncer'
} elseif ($IsLinux) {
  $platform = 'linux'
  $extension = '.tar.gz'
  $executable = 'build-bouncer'
} else {
  throw 'Unsupported operating system'
}

$architecture = switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()) {
  'X64' { 'amd64' }
  'Arm64' { 'arm64' }
  default { throw 'Unsupported architecture' }
}

$archiveName = "build-bouncer_$($version)_$($platform)_$($architecture)$($extension)"
$archivePath = Join-Path $dist $archiveName
$checksumsPath = Join-Path $dist "build-bouncer_$($version)_checksums.txt"

$entries = @(Get-Content -LiteralPath $checksumsPath | Where-Object {
  $_.Trim().EndsWith("  $archiveName") -or $_.Trim().EndsWith(" *$archiveName")
})
if ($entries.Count -ne 1) {
  throw "Missing or duplicate checksum for $archiveName"
}
$expectedHash = ($entries[0].Trim() -split '\s+')[0].ToLowerInvariant()
$actualHash = (Get-FileHash -LiteralPath $archivePath -Algorithm SHA256).Hash.ToLowerInvariant()
if ($expectedHash -ne $actualHash) {
  throw "Checksum mismatch for $archiveName"
}

$destination = Join-Path ([System.IO.Path]::GetTempPath()) ("build-bouncer-smoke-" + [guid]::NewGuid())
New-Item -ItemType Directory -Path $destination | Out-Null
try {
  if ($IsWindows) {
    Expand-Archive -LiteralPath $archivePath -DestinationPath $destination
  } else {
    & tar -xzf $archivePath -C $destination
    if ($LASTEXITCODE -ne 0) {
      throw "Failed to extract $archiveName"
    }
  }

  $binary = Join-Path $destination $executable
  if (-not (Test-Path -LiteralPath $binary -PathType Leaf)) {
    throw "Archive does not contain $executable"
  }

  $reported = (& $binary --version | Out-String).Trim()
  if ($LASTEXITCODE -ne 0 -or -not $reported.StartsWith("build-bouncer v$version ")) {
    throw "Installed binary reported an incorrect version: $reported"
  }
  if (-not [string]::IsNullOrWhiteSpace([string]$metadata.commit) -and
      -not $reported.Contains([string]$metadata.commit)) {
    throw "Installed binary does not report release commit $($metadata.commit)"
  }

  Write-Host "Verified $archiveName ($actualHash)"
  Write-Host $reported
} finally {
  Remove-Item -LiteralPath $destination -Force -Recurse
}
