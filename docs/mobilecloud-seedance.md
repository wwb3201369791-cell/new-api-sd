# 移动云 Seedance 适配

## 对外接口

客户端继续使用火山方舟 Seedance 兼容接口，不需要感知实际上游：

- `POST /v1/videos`、`GET /v1/videos/:task_id`
- `GET /v1/videos`、`DELETE /v1/videos/:task_id`
- `POST /api/v3/contents/generations/tasks`
- `GET /api/v3/contents/generations/tasks/:task_id`
- `GET /api/v3/contents/generations/tasks`
- `DELETE /api/v3/contents/generations/tasks/:task_id`
- `GET|HEAD /v1/videos/:task_id/content`

任务创建、轮询、结果代理、列表、取消/删除均在网关完成。取消会使用
Compare-And-Swap 更新本地任务并执行一次额度退款，避免轮询器并发重复退款。

每个响应都会带 `X-Oneapi-Request-Id`；若上游提供请求 ID，同时返回
`X-Upstream-Request-Id`。上游返回的请求 ID 会写入管理员任务
详情和日志的 `upstream_request_id` 字段；任务创建成功后还会持久化上游任务
ID，因而可以用“网关请求 ID → 上游请求 ID → 任务 ID”定位一次完整调用。

## 渠道配置

在管理后台新增一个“任务插件”渠道：

1. 任务插件选择 `Mobile Cloud Seedance`，模型选择 `doubao-seedance-2.0`。
2. 基础 URL 填移动云模型服务区域地址，例如 `https://zhenze-huhehaote.cmecloud.cn`，不要追加 `/api/v3`。
3. API 密钥填写移动云模型服务的 `MAAS_API_KEY`（Bearer key）。
4. 如需素材库，在渠道高级设置 → 移动云素材库中启用开关并填写凭证。
   需要通过接口或脚本写入时，对应的 `setting` JSON 为：

```json
{
  "task_plugin_key": "mobilecloud",
  "asset_enabled": true,
  "asset_base_url": "https://ecloud.10086.cn",
  "asset_access_key": "ACCESS_KEY",
  "asset_secret_key": "SECRET_KEY",
  "asset_resource_pool": "CIDC-CORE-00"
}
```

这些字段可在渠道编辑器的“高级设置 → 移动云素材库”中填写。素材 Access Key
和 Secret Key 输入框为只写模式：编辑已有渠道时留空即可保留已保存的值。关闭
开关会保留凭证，但会阻止素材管理请求。

素材 AK/SK 与视频生成 Bearer key 分开保存，服务端不会返回给用户。

保存渠道后，管理员可以在同一处点击“测试素材库连接”。该检查只调用上游
`ListAssetGroups` 读取一个素材组，不会创建、上传或删除任何资源。对应管理
接口为 `GET /api/channel/asset-test/:id`（需要渠道操作权限），成功响应只
包含渠道、提供商、状态码和可选的上游请求 ID，不返回 AK/SK。关闭素材库或
未填写完整 AK/SK 时，测试按钮会保持禁用，避免把视频 Bearer key 误当成素材
凭证。

### 默认模型计费

`doubao-seedance-2.0`（以及上游别名 `doubao-seedance-2-0-260128`）内置了
按上游 completion token 结算的阶梯表达式：无输入视频时 480p/720p 为 92
元/百万 token、1080p 为 102 元/百万 token；有输入视频时分别为 56 和 62
元/百万 token。管理员仍可在分组与模型定价设置中覆盖这组默认值，覆盖后以
数据库中的表达式为准。

### 制品预览与公网地址

视频/音频/图片预览请求会显式带 `disposition=inline`，下载请求带
`disposition=attachment`，网关不会把上游的 `Content-Disposition: attachment`
误传给播放器。生产环境应把系统设置中的 `TaskPublicAddress` 配置为客户端
可访问的完整 HTTPS 地址（例如 `https://api.example.com`，不要带路径查询
参数）；未配置时才回退到 `ServerAddress`。反向代理终止 TLS 时也必须设置该
地址，否则浏览器会按混合内容规则拦截 HTTP 制品链接。

## 素材管理 API

用户令牌调用以下网关接口即可管理素材组和素材：

- `GET|POST /api/mobilecloud/asset-groups`
- `GET|PUT|DELETE /api/mobilecloud/asset-groups/:group_id`
- `GET|POST /api/mobilecloud/assets`
- `GET|PUT|DELETE /api/mobilecloud/assets/:asset_id`
- `POST /api/mobilecloud/real-person-auth/sessions`
- `POST /api/mobilecloud/real-person-auth/asset-group/by-byted-token`
- `POST /api/mobilecloud/billing/tokens/consumed`
- `POST /api/mobilecloud/billing/deductions`
- `POST /api/mobilecloud/billing/deductions/export`
- `GET /api/mobilecloud/billing/deductions/export/:task_id`

以上素材组/素材接口同时提供 `/v1/asset-groups` 和 `/v1/assets` 厂商无关别名。
每个客户的默认 AIGC 素材组按需自动创建；创建素材时省略 `groupId` 会自动
使用该默认组。显式传入的组 ID 必须属于当前客户和当前渠道，其他客户的资源
会返回 404。

网关按移动云 V2.0 规则签名（HMAC-SHA1，支持 HMAC-SHA256），并保留一份
脱敏后的本地索引。生产 ToB 模式建议只提交公网 URL；网关默认关闭 multipart
上传。确需网页端代传时，显式设置 `ASSET_STORAGE_ALLOW_UPLOAD=true`，此时
`POST /api/mobilecloud/uploads` 才会将本地图片、视频或音频写入本地目录或
S3 兼容对象存储，再以公网 URL 注册到移动云。对象存储由
`ASSET_STORAGE_MODE=local|s3` 选择，S3 模式兼容 MinIO、R2、OSS 等服务。
移动云会将对象下载到其 EOS 存储并返回约 12 小时有效的预签名 URL；真人
素材组必须经过官方活体认证流程，网关只转发认证会话和 Token 查询，不绕过
认证。资费查询/导出接口也已透传，但 New API 的客户额度结算仍以自身账单
系统为准。

移动云 V2.0 的时间字段按官方 SDK 约定使用北京时间墙上时钟并保留 `Z` 后缀；
网关已固定该格式，部署主机使用 UTC 时也无需手动调整时区。素材请求默认使用
HTTP/1.1，避免移动云素材网关在 HTTP/2 下提前关闭连接。

## 错误、超时与幂等

上游错误会保留在管理员日志（敏感字段脱敏、正文截断），对客户返回稳定的
语义错误：503→409 `upstream_busy`、501→422 `feature_unavailable`、网关或
上游超时→409 `upstream_timeout`、401/403→凭证错误、429→429 `rate_limited`。
任务创建的单次上游请求默认 120 秒，可通过
`TASK_UPSTREAM_TIMEOUT_SECONDS` 调整。创建阶段按现有 `RETRY_TIMES` 对 429/5xx
重试；轮询读取对传输错误、408、429、5xx 做最多 3 次短退避重试。

客户端可在 `POST /v1/videos` 或 Ark 创建接口带 `Idempotency-Key`（最长 255
字符）。相同用户、路径和 key 在 24 小时内只会产生一个任务；并发重复请求返回
`idempotency_in_progress`，已完成请求会重放原任务响应。Redis 部署使用 Redis
原子锁，未启用 Redis 时自动退回单节点内存锁。

## 润元扩展

对外协议、任务模型、素材 API 的网关边界已经独立。接入润元时新增一个
provider 插件，复用列表/取消/删除/素材控制器，只实现其鉴权、请求字段、
状态映射和结果 URL 适配即可；客户端 URL 不变。

## 验收

没有真实移动云 AK/SK 和可访问素材 URL 时只能完成协议与签名单元测试，不能
替代真实计费任务的端到端验收。上线前应使用测试账户验证：创建任务、轮询成功、
视频代理、取消退款、创建素材、查询素材和预签名 URL 访问。

移动云素材库的上游直连查询、创建、精确查询和清理命令，见
[移动云素材库 curl 测试指南](mobilecloud-asset-curl.md)。

可直接运行 `pwsh -File e2e/mobilecloud-seedance.ps1 -BaseUrl
https://HOST -Token TOKEN` 做真实网关验收。脚本不会接触或输出移动云密钥；
移动云 Bearer key 必须先在后台的 `mobilecloud` 渠道中配置。

## 网关素材 API 一键烟测

Windows 可运行：

```powershell
cd new-api\scripts
.\run_mobilecloud_gateway_asset_test.bat
```

或直接运行 PowerShell：

```powershell
pwsh -File .\mobilecloud_gateway_asset_test.ps1 `
  -BaseUrl "http://127.0.0.1:3000" `
  -ChannelId CHANNEL_ID
```

脚本会安全提示输入 New API 客户密钥，并依次验证：默认素材组、创建自定义
素材组、素材组详情、素材组更新，以及（提供公网 `-AssetUrl` 时）创建素材、
查询素材列表和素材详情。默认创建的测试组会保留，便于继续手工调试；确认
完成后加 `-Cleanup` 删除脚本创建的测试组和素材：

```powershell
pwsh -File .\mobilecloud_gateway_asset_test.ps1 `
  -BaseUrl "https://HOST" `
  -Token TOKEN `
  -AssetUrl "https://PUBLIC_URL/demo.png" `
  -Cleanup
```

脚本同时在系统临时目录生成三张极小 PNG 作为本地夹具，测试结束自动清理。
移动云会从 `assetUrl` 下载素材，因此本地文件、`localhost`、`127.0.0.1` 或
内网地址不能直接用于上游素材注册；要完成真实素材测试，请传入移动云可访问
的公网 HTTP(S) 地址。`-SkipAsset` 可只测试素材组流程。
