<#!
.SYNOPSIS
  End-to-end smoke test for the provider-neutral Mobile Cloud asset API.

.DESCRIPTION
  Creates a customer-scoped custom asset group, verifies that the default
  group is present, optionally registers a publicly reachable asset URL, and
  exercises list/detail/update/delete operations.  The script only accepts a
  customer-facing New API token.  Mobile Cloud AccessKey/SecretKey remain in
  the channel configuration and are never requested or printed here.

  A local PNG fixture is generated under the process temporary directory for
  reference.  Mobile Cloud fetches assetUrl asynchronously, so a local file
  is not registered unless it is exposed through a public HTTP(S) URL.  Pass
  -AssetUrl with such a URL to run the provider asset checks.
#>
[CmdletBinding()]
param(
  [string]$BaseUrl = "http://127.0.0.1:3000",
  [string]$Token = "",
  [string]$ChannelId = "",
  [string]$AssetUrl = "https://httpbin.org/image/png",
  [switch]$SkipAsset,
  [switch]$Cleanup,
  [switch]$KeepFixtures
)

$ErrorActionPreference = "Stop"

function Read-NewApiToken {
  $secure = Read-Host "输入 New API Token（sk-...）" -AsSecureString
  $ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
  try {
    return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr)
  } finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)
  }
}

function Invoke-GatewayJson {
  param(
    [Parameter(Mandatory = $true)][ValidateSet("GET", "POST", "PUT", "DELETE")][string]$Method,
    [Parameter(Mandatory = $true)][string]$Path,
    [object]$Body,
    [string]$IdempotencyKey = ""
  )

  $headers = @{ Authorization = "Bearer $script:Token" }
  if ($IdempotencyKey) { $headers["Idempotency-Key"] = $IdempotencyKey }
  $uri = "$script:BaseUrl$Path"
  try {
    if ($null -ne $Body) {
      $json = $Body | ConvertTo-Json -Depth 20 -Compress
      return Invoke-RestMethod -Method $Method -Uri $uri -Headers $headers -ContentType "application/json" -Body $json
    }
    return Invoke-RestMethod -Method $Method -Uri $uri -Headers $headers
  } catch {
    $status = "unknown"
    $detail = $_.Exception.Message
    if ($_.Exception.Response) {
      try { $status = [int]$_.Exception.Response.StatusCode } catch {}
      try {
        $reader = New-Object IO.StreamReader($_.Exception.Response.GetResponseStream())
        $detail = $reader.ReadToEnd()
        $reader.Dispose()
      } catch {}
    }
    throw "${Method} ${Path} failed (HTTP ${status}): ${detail}"
  }
}

function Add-ChannelQuery {
  param([Parameter(Mandatory = $true)][string]$Path)
  if ([string]::IsNullOrWhiteSpace($script:ChannelId)) { return $Path }
  $separator = if ($Path.Contains("?")) { "&" } else { "?" }
  return "$Path${separator}channel_id=$([uri]::EscapeDataString($script:ChannelId))"
}

function Get-ProviderBody {
  param([Parameter(Mandatory = $true)]$Response)
  if ($null -eq $Response.data) { throw "网关响应缺少 data 字段" }
  if ($null -eq $Response.data.body) { throw "上游响应缺少 body 字段: $($Response | ConvertTo-Json -Depth 10 -Compress)" }
  return $Response.data.body
}

function Get-ProviderItems {
  param([Parameter(Mandatory = $true)]$Body)
  foreach ($key in @("data", "list", "items", "results", "records")) {
    $value = $Body.$key
    if ($null -ne $value) {
      if ($value -is [System.Array]) { return @($value) }
      return @($value)
    }
  }
  return @()
}

function Get-ProviderId {
  param([Parameter(Mandatory = $true)]$Value, [Parameter(Mandatory = $true)][string[]]$Keys)
  if ($null -eq $Value) { return "" }
  if ($Value -is [string]) { return $Value.Trim() }
  if ($Value -is [System.Collections.IDictionary] -or $Value.PSObject.Properties.Count -gt 0) {
    foreach ($key in $Keys) {
      $candidate = $Value.$key
      if ($candidate -is [string] -and -not [string]::IsNullOrWhiteSpace($candidate)) { return $candidate.Trim() }
    }
    foreach ($key in @("body", "data", "result", "Result", "resource")) {
      $nested = $Value.$key
      $id = Get-ProviderId -Value $nested -Keys $Keys
      if ($id) { return $id }
    }
  }
  if ($Value -is [System.Array]) {
    foreach ($item in $Value) {
      $id = Get-ProviderId -Value $item -Keys $Keys
      if ($id) { return $id }
    }
  }
  return ""
}

function Write-Step {
  param([string]$Name, [string]$Status, [string]$Detail = "")
  $color = if ($Status -eq "PASS") { "Green" } elseif ($Status -eq "SKIP") { "Yellow" } else { "Red" }
  $suffix = if ($Detail) { " - $Detail" } else { "" }
  Write-Host ("[{0}] {1}{2}" -f $Status, $Name, $suffix) -ForegroundColor $color
}

if ([string]::IsNullOrWhiteSpace($Token)) {
  if (-not [string]::IsNullOrWhiteSpace($env:NEW_API_TOKEN)) {
    $Token = $env:NEW_API_TOKEN
  } else {
    $Token = Read-NewApiToken
  }
}
if ([string]::IsNullOrWhiteSpace($Token)) { throw "New API Token 不能为空" }

$script:BaseUrl = $BaseUrl.TrimEnd('/')
$script:Token = $Token.Trim()
$runId = Get-Date -Format "yyyyMMdd-HHmmss"
$groupName = "new-api-e2e-$runId"
$fixtureRoot = Join-Path ([IO.Path]::GetTempPath()) "new-api-mobilecloud-$runId"
$createdGroupId = ""
$createdAssetIds = [System.Collections.Generic.List[string]]::new()
$fixtureCreated = $false

try {
  # Tiny valid PNG.  The fixture is deliberately generated outside the repo
  # so repeated smoke tests do not consume workspace/U-disk space.
  New-Item -ItemType Directory -Path $fixtureRoot -Force | Out-Null
  $png = [Convert]::FromBase64String("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")
  1..3 | ForEach-Object { [IO.File]::WriteAllBytes((Join-Path $fixtureRoot "asset-$_.png"), $png) }
  $fixtureCreated = $true
  Write-Step "生成本地测试图片" "PASS" $fixtureRoot

  $groupsPath = Add-ChannelQuery "/v1/asset-groups"
  $groupsResponse = Invoke-GatewayJson -Method GET -Path $groupsPath
  $groupsBody = Get-ProviderBody $groupsResponse
  $groups = Get-ProviderItems $groupsBody
  $defaultGroup = $groups | Where-Object { $_.groupName -like "customer-*-default" } | Select-Object -First 1
  $defaultGroupId = Get-ProviderId -Value $defaultGroup -Keys @("groupId", "groupID", "GroupId", "Id")
  if (-not $defaultGroupId) { throw "查询成功但未找到默认素材组" }
  Write-Step "查询并确认默认素材组" "PASS" $defaultGroupId

  $created = Invoke-GatewayJson -Method POST -Path $groupsPath -Body @{
    groupName = $groupName
    description = "temporary New API Mobile Cloud E2E group"
  }
  $createdBody = Get-ProviderBody $created
  $createdGroupId = Get-ProviderId -Value $createdBody -Keys @("groupId", "groupID", "GroupId", "Id")
  if (-not $createdGroupId) { throw "创建素材组成功响应中没有 groupId" }
  Write-Step "创建自定义素材组" "PASS" "$createdGroupId ($groupName)"

  $groupDetail = Invoke-GatewayJson -Method GET -Path (Add-ChannelQuery "/v1/asset-groups/$([uri]::EscapeDataString($createdGroupId))")
  $detailId = Get-ProviderId -Value (Get-ProviderBody $groupDetail) -Keys @("groupId", "groupID", "GroupId", "Id")
  if ($detailId -ne $createdGroupId) { throw "素材组详情 ID 不匹配: $detailId" }
  Write-Step "查询自定义素材组详情" "PASS" $detailId

  $updatedName = "$groupName-updated"
  $updated = Invoke-GatewayJson -Method PUT -Path (Add-ChannelQuery "/v1/asset-groups/$([uri]::EscapeDataString($createdGroupId))") -Body @{
    groupName = $updatedName
    description = "updated temporary New API Mobile Cloud E2E group"
  }
  $updatedBody = Get-ProviderBody $updated
  $returnedName = [string]$updatedBody.groupName
  if ($returnedName -and $returnedName -ne $updatedName) { throw "素材组更新名称不匹配: $returnedName" }
  Write-Step "更新自定义素材组" "PASS" $updatedName

  if ($SkipAsset) {
    Write-Step "创建/查询素材" "SKIP" "已通过 -SkipAsset 跳过"
  } elseif ([string]::IsNullOrWhiteSpace($AssetUrl)) {
    Write-Step "创建/查询素材" "SKIP" "未提供公网 AssetUrl；本地图片不能直接被移动云抓取"
  } else {
    $assetResponse = Invoke-GatewayJson -Method POST -Path (Add-ChannelQuery "/v1/assets") -Body @{
      assetName = "new-api-e2e-$runId"
      assetType = "Image"
      assetUrl = $AssetUrl
      groupId = $createdGroupId
    }
    $assetBody = Get-ProviderBody $assetResponse
    $assetId = Get-ProviderId -Value $assetBody -Keys @("assetId", "assetID", "AssetId", "Id")
    if (-not $assetId) { throw "创建素材成功响应中没有 assetId" }
    $createdAssetIds.Add($assetId)
    Write-Step "注册公网素材" "PASS" $assetId

    $assetsPath = Add-ChannelQuery "/v1/assets?group_id=$([uri]::EscapeDataString($createdGroupId))"
    $assetsResponse = Invoke-GatewayJson -Method GET -Path $assetsPath
    $assetsBody = Get-ProviderBody $assetsResponse
    $assets = Get-ProviderItems $assetsBody
    $listed = $assets | Where-Object { (Get-ProviderId -Value $_ -Keys @("assetId", "assetID", "AssetId", "Id")) -eq $assetId } | Select-Object -First 1
    if ($null -eq $listed) { throw "素材创建成功但列表中未找到 $assetId" }
    Write-Step "查询素材列表" "PASS" $assetId

    $assetDetailResponse = Invoke-GatewayJson -Method GET -Path (Add-ChannelQuery "/v1/assets/$([uri]::EscapeDataString($assetId))")
    $assetDetailId = Get-ProviderId -Value (Get-ProviderBody $assetDetailResponse) -Keys @("assetId", "assetID", "AssetId", "Id")
    if ($assetDetailId -ne $assetId) { throw "素材详情 ID 不匹配: $assetDetailId" }
    Write-Step "查询素材详情" "PASS" $assetDetailId
  }

  if ($Cleanup) {
    foreach ($assetId in $createdAssetIds) {
      Invoke-GatewayJson -Method DELETE -Path (Add-ChannelQuery "/v1/assets/$([uri]::EscapeDataString($assetId))") | Out-Null
      Write-Step "清理测试素材" "PASS" $assetId
    }
    if ($createdGroupId) {
      Invoke-GatewayJson -Method DELETE -Path (Add-ChannelQuery "/v1/asset-groups/$([uri]::EscapeDataString($createdGroupId))") | Out-Null
      Write-Step "清理测试素材组" "PASS" $createdGroupId
    }
  } else {
    Write-Step "清理测试资源" "SKIP" "未指定 -Cleanup，保留测试组 $createdGroupId"
  }

  Write-Host ""
  Write-Host "移动云素材 API 烟测完成。" -ForegroundColor Green
  Write-Host "默认组: $defaultGroupId"
  Write-Host "测试组: $createdGroupId"
  if ($createdAssetIds.Count -gt 0) { Write-Host "测试素材: $($createdAssetIds -join ', ')" }
  Write-Host "注意：素材 URL 必须是移动云可访问的公网 HTTP(S) 地址。"
} catch {
  if ($Cleanup) {
    foreach ($assetId in $createdAssetIds) {
      try {
        Invoke-GatewayJson -Method DELETE -Path (Add-ChannelQuery "/v1/assets/$([uri]::EscapeDataString($assetId))") | Out-Null
        Write-Step "失败后清理测试素材" "PASS" $assetId
      } catch {
        Write-Step "失败后清理测试素材" "FAIL" "$assetId ($($_.Exception.Message))"
      }
    }
    if ($createdGroupId) {
      try {
        Invoke-GatewayJson -Method DELETE -Path (Add-ChannelQuery "/v1/asset-groups/$([uri]::EscapeDataString($createdGroupId))") | Out-Null
        Write-Step "失败后清理测试素材组" "PASS" $createdGroupId
      } catch {
        Write-Step "失败后清理测试素材组" "FAIL" "$createdGroupId ($($_.Exception.Message))"
      }
    }
  }
  throw
} finally {
  if ($fixtureCreated -and -not $KeepFixtures) {
    Remove-Item -LiteralPath $fixtureRoot -Recurse -Force -ErrorAction SilentlyContinue
  } elseif ($fixtureCreated) {
    Write-Host "本地测试图片保留在: $fixtureRoot" -ForegroundColor Yellow
  }
  # Do not retain the token in the script scope after the test.
  $script:Token = $null
}
