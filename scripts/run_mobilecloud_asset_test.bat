@echo off
setlocal
cd /d "%~dp0"
where py >nul 2>nul
if %errorlevel%==0 (
  py -3 mobilecloud_asset_test.py %*
) else (
  python mobilecloud_asset_test.py %*
)
echo.
pause
