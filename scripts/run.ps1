# Run cursor2api on Windows
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$Binary = Join-Path $Root "cursor2api.exe"
$Config = Join-Path $Root "config.json"
$Example = Join-Path $Root "config.example.json"

if (-not (Test-Path $Config)) {
  Copy-Item $Example $Config
  Write-Host "已创建 $Config"
}

if (-not (Test-Path $Binary)) {
  Write-Host "正在编译 cursor2api..."
  Push-Location $Root
  go build -o cursor2api.exe ./src
  Pop-Location
}

& $Binary $Config
