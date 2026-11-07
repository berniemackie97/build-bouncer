param(
  [Parameter(Mandatory = $true)]
  [string]$DistDir
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$dist = (Resolve-Path -LiteralPath $DistDir).Path
$metadata = Get-Content -Raw -LiteralPath (Join-Path $dist 'metadata.json') | ConvertFrom-Json
$version = [string]$metadata.version
if ([string]::IsNullOrWhiteSpace($version)) {
  throw 'Release metadata has no version'
}

$checksumsPath = Join-Path $dist 'checksums.txt'
$checksums = @{}
foreach ($line in Get-Content -LiteralPath $checksumsPath) {
  if ([string]::IsNullOrWhiteSpace($line)) {
    continue
  }
  if ($line.Trim() -notmatch '^([a-fA-F0-9]{64})\s+\*?([^/\s\\]+)$') {
    throw "Invalid checksum entry: $line"
  }
  $hash = $Matches[1].ToLowerInvariant()
  $name = $Matches[2]
  if ($checksums.ContainsKey($name)) {
    throw "Duplicate checksum for $name"
  }
  $checksums[$name] = $hash
}

foreach ($os in @('darwin', 'linux', 'windows')) {
  foreach ($arch in @('amd64', 'arm64')) {
    $extension = if ($os -eq 'windows') { '.zip' } else { '.tar.gz' }
    $name = "build-bouncer_$($version)_$($os)_$($arch)$extension"
    if (-not $checksums.ContainsKey($name)) {
      throw "Missing checksum for $name"
    }
    $archive = Join-Path $dist $name
    if (-not (Test-Path -LiteralPath $archive -PathType Leaf)) {
      throw "Missing archive $name"
    }
    $actual = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $checksums[$name]) {
      throw "Checksum mismatch for $name"
    }
  }
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

$archiveName = "build-bouncer_$($version)_$($platform)_$($architecture)$extension"
$archivePath = Join-Path $dist $archiveName
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
  & $binary --help | Out-Null
  if ($LASTEXITCODE -ne 0) {
    throw 'Installed binary failed the CLI help smoke test'
  }

  Write-Host "Verified all six archives and installed $archiveName"
  Write-Host $reported
} finally {
  Remove-Item -LiteralPath $destination -Force -Recurse
}
