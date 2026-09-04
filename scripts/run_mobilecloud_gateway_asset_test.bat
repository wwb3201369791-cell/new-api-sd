@echo off
setlocal
cd /d "%~dp0"
where pwsh >nul 2>nul
if %errorlevel%==0 (
  pwsh -NoProfile -ExecutionPolicy Bypass -File mobilecloud_gateway_asset_test.ps1 %*
) else (
  powershell -NoProfile -ExecutionPolicy Bypass -File mobilecloud_gateway_asset_test.ps1 %*
)
exit /b %errorlevel%
