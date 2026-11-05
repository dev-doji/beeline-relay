<#
.SYNOPSIS
  Starts the Beeline API (port 8080) and website (port 3001) in two windows that stay open.

.DESCRIPTION
  Uses beeline-api/.env for the API settings. Close a window (or press Ctrl+C in it) to stop
  that server. Then open http://localhost:3001 in your browser.

.EXAMPLE
  .\scripts\dev.ps1
#>
$Root = Split-Path -Parent $PSScriptRoot

Start-Process powershell -ArgumentList @(
  '-NoExit', '-Command',
  "Set-Location '$Root\beeline-api'; `$host.UI.RawUI.WindowTitle = 'Beeline API :8080'; go run ./cmd/api"
)
Start-Process powershell -ArgumentList @(
  '-NoExit', '-Command',
  "Set-Location '$Root\beeline-web'; `$host.UI.RawUI.WindowTitle = 'Beeline web :3001'; if (-not (Test-Path node_modules)) { pnpm install }; pnpm exec next dev -p 3001"
)

Write-Host 'Starting the API and the website in two new windows...' -ForegroundColor Cyan
Write-Host 'In about 20 seconds, open http://localhost:3001 (Vendors: http://localhost:3001/vendor)' -ForegroundColor Cyan
