$ErrorActionPreference = 'Stop'
$script = Join-Path $PSScriptRoot 'mobilecloud_asset_test.py'
if (Get-Command py -ErrorAction SilentlyContinue) {
    & py -3 $script @args
} else {
    & python $script @args
}
exit $LASTEXITCODE
