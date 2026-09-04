# 移动云 Seedance 使用与验收说明

本文是给 **管理员** 和 **API 用户** 的操作手册。网关对外提供统一的
Seedance/素材 API，用户只需要 New API Token；移动云的 Bearer key、素材
AccessKey/SecretKey 只由管理员保存在对应渠道中，不能交给客户或写入客户端。

## 一、管理员配置

### 1. 创建移动云视频渠道

在管理后台新建“任务插件”渠道：

| 配置项 | 填写内容 |
| --- | --- |
| 任务插件 | `Mobile Cloud Seedance`（key：`mobilecloud`） |
| 模型 | `doubao-seedance-2.0` |
| 基础 URL | 移动云模型服务区域地址，例如 `https://MODEL_ENDPOINT` |
| API 密钥 | 移动云模型服务签发的 MAAS Bearer key |

基础 URL 只填上游模型服务的域名，不要追加 `/api/v3`。网关会自动请求
`/api/v3/contents/generations/tasks`。视频生成凭证与素材库凭证是两套独立
凭证，不能互换。

### 2. 配置素材库（可选）

在渠道编辑器的“高级设置 → 移动云素材库”打开开关，填写移动云账号侧签发的
AK/SK：

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

保存后点击“测试素材库连接”。该按钮只读一个素材组，不会创建、上传或删除
素材。输入框是只写模式，编辑渠道时留空表示保留已保存的 SecretKey。关闭
开关会保留凭证，但会阻止素材 API 请求。

### 3. 创建客户 Token 与路由

1. 在“API 密钥”中为客户创建 New API Token，并授予
   `doubao-seedance-2.0` 模型和对应分组。
2. 在渠道列表确认移动云渠道已启用、模型映射正确，并设置权重/故障切换策略。
3. 多个移动云渠道同时启用时，素材接口可加 `?channel_id=CHANNEL_ID` 指定
   渠道；不传时网关选择第一个已启用且配置完整素材凭证的移动云/润元渠道。
4. 生产环境把系统 `TaskPublicAddress` 配置为客户端可访问的 HTTPS 地址，例如
   `https://API_DOMAIN`。否则浏览器在 HTTPS 页面中可能拦截 HTTP 视频制品链接。

## 二、用户调用视频接口

用户只使用网关地址和自己的 New API Token，不需要知道实际转发到移动云：

```powershell
$BaseUrl = "https://HOST"
$Token = "TOKEN"
$body = @{
  model = "doubao-seedance-2.0"
  content = @(@{ type = "text"; text = "一只猫在海边奔跑，电影感，夕阳" })
  duration = 5
  resolution = "720p"
  ratio = "16:9"
} | ConvertTo-Json -Depth 10 -Compress

curl.exe -i -X POST "$BaseUrl/v1/videos" `
  -H "Authorization: Bearer $Token" `
  -H "Content-Type: application/json" `
  --data-raw $body
```

记录返回的 `id`（即 `TASK_ID`），然后轮询：

```powershell
curl.exe -s "$BaseUrl/v1/videos/TASK_ID" `
  -H "Authorization: Bearer $Token"
```

状态通常按 `queued → in_progress → completed` 变化；`failed` 表示上游任务
失败。成功后使用网关制品地址播放或下载：

```powershell
curl.exe -L -o .\result.mp4 "$BaseUrl/v1/videos/TASK_ID/content" `
  -H "Authorization: Bearer $Token"
```

也支持火山方舟风格路径 `/api/v3/contents/generations/tasks`，请求体保持
`model`、`content`、`duration`、`resolution`、`ratio` 字段不变。列表和删除
分别是 `GET /v1/videos`、`DELETE /v1/videos/TASK_ID`。

### 图生视频

移动云需要上游可以访问的公网 HTTP(S) 地址：

```json
{
  "model": "doubao-seedance-2.0",
  "content": [
    {"type": "image_url", "image_url": {"url": "https://PUBLIC_URL/image.png"}},
    {"type": "text", "text": "镜头缓慢推进，保持主体外观一致"}
  ],
  "duration": 5,
  "resolution": "720p",
  "ratio": "16:9"
}
```

`localhost`、`127.0.0.1`、内网地址或需要登录的地址不能作为素材 URL；网关
默认不会保存公网 URL 对应的文件。

## 三、用户调用素材接口

素材接口使用同一个 New API Token。每个客户在每个渠道下都有独立的默认
`AIGC` 素材组；首次查询或注册素材时按需创建。客户不传 `groupId` 时自动使用
自己的默认组，传入组 ID 时网关会校验“当前客户 + 当前渠道”的归属。

### 1. 查询默认素材组

```powershell
curl.exe -i "$BaseUrl/v1/asset-groups" `
  -H "Authorization: Bearer $Token"
```

多渠道场景：

```powershell
curl.exe -i "$BaseUrl/v1/asset-groups?channel_id=CHANNEL_ID" `
  -H "Authorization: Bearer $Token"
```

### 2. 创建、查询和删除自定义素材组

```powershell
curl.exe -i -X POST "$BaseUrl/v1/asset-groups" `
  -H "Authorization: Bearer $Token" `
  -H "Content-Type: application/json" `
  --data-raw '{"groupName":"customer-project-001","description":"项目素材"}'
```

返回值中的 `data.body.groupId` 是 `GROUP_ID`。后续接口：

```text
GET    /v1/asset-groups/GROUP_ID
PUT    /v1/asset-groups/GROUP_ID       body: {"groupName":"new-name"}
DELETE /v1/asset-groups/GROUP_ID
```

默认组不能删除；同一客户/渠道下重复的 `groupName` 返回
`409 ASSET_GROUP_NAME_EXISTS`。

### 3. 注册和管理素材

```powershell
# 不传 groupId：自动进入当前客户的默认组
curl.exe -i -X POST "$BaseUrl/v1/assets" `
  -H "Authorization: Bearer $Token" `
  -H "Content-Type: application/json" `
  --data-raw '{"assetName":"demo","assetType":"Image","assetUrl":"https://PUBLIC_URL/demo.png"}'

# 指定自定义素材组
curl.exe -i -X POST "$BaseUrl/v1/assets" `
  -H "Authorization: Bearer $Token" `
  -H "Content-Type: application/json" `
  --data-raw '{"assetName":"demo-2","assetType":"Image","assetUrl":"https://PUBLIC_URL/demo-2.png","groupId":"GROUP_ID"}'
```

返回值中的 `data.body.assetId` 是 `ASSET_ID`。素材处理是异步的，先列表或
详情轮询到 `status=ACTIVE` 后再用于视频任务：

```text
GET    /v1/assets?group_id=GROUP_ID
GET    /v1/assets/ASSET_ID
PUT    /v1/assets/ASSET_ID       body: {"assetName":"new-name"}
DELETE /v1/assets/ASSET_ID
```

如果暂时没有公网素材 URL，只能先验证素材组接口；不要把本地生成的 PNG 直接
作为 `assetUrl`。需要网页代传文件时，管理员必须显式开启
`ASSET_STORAGE_ALLOW_UPLOAD=true`，并配置本地或 S3 对象存储。

## 四、一键回归测试

仓库自带网关素材 API 烟测脚本。它只要求客户 New API Token，不会询问或输出
移动云 AK/SK：

```powershell
cd new-api\scripts
.\run_mobilecloud_gateway_asset_test.bat `
  -BaseUrl "https://HOST" `
  -Token "TOKEN" `
  -AssetUrl "https://PUBLIC_URL/demo.png" `
  -TestDefaultGroup `
  -TestDuplicateGroupName `
  -Cleanup
```

脚本覆盖：默认组、创建/详情/更新自定义组、重复名称保护、注册素材、列表、
详情状态轮询、更新素材名称和清理。`-SkipAsset` 可只测素材组，`-Cleanup` 会
删除本次创建的测试资源。

## 五、管理员排错顺序

1. **401/403**：先确认客户端 New API Token 有效；再检查渠道中的视频 Bearer
   key 或素材 AK/SK，二者不能混用。
2. **400 `ASSET_PROVIDER_ERROR` 且提示下载 URL 失败**：移动云已收到请求，
   但无法从 `assetUrl` 下载文件。检查公网 DNS、HTTPS 证书、重定向、响应体和
   防火墙，不是素材组归属问题。
3. **429**：上游限流，降低并发并等待重试；不要在客户端重复提交没有
   `Idempotency-Key` 的创建请求。
4. **503/501/超时**：网关会转换为语义化错误（上游繁忙、功能不可用、上游超时），
   同时保留管理员日志中的原始状态和错误详情。
5. **视频预览失败**：先用 `GET /v1/videos/TASK_ID/content` 下载验证，再检查
   `TaskPublicAddress` 是否为浏览器可访问的 HTTPS 地址。

每次请求都可以用响应头 `X-Oneapi-Request-Id` 定位；若上游返回请求 ID，还会有
`X-Upstream-Request-Id`。管理员日志中可按“网关请求 ID → 上游请求 ID → 任务 ID”
串起完整链路。

## 六、测试结论判定

- 渠道测试成功：只证明渠道凭证、路由和上游入口可达。
- 素材烟测全部通过：证明网关素材组/素材 CRUD、默认组回退和客户隔离正常。
- 视频创建、轮询、制品下载全部通过：才算移动云 Seedance 端到端验收完成。
- 真实计费、并发限流、出口 IP 白名单和生产 HTTPS 仍需在上线环境各做一次验收。
