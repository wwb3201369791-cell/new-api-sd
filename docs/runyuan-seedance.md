# 润元 Seedance 渠道

润元渠道通过任务插件接入。对外仍使用 New API 的统一视频接口，调用方不需要区分润元和其他上游；管理员在渠道配置中选择不同的任务插件即可切换上游。

## 当前支持范围

- `POST /v1/videos`、`GET /v1/videos/{task_id}`；网关提供本地任务删除/取消语义，润元文档当前未公布上游任务 DELETE 接口，因此不会伪造上游取消请求。
- `POST /v1/responses` 的视频任务模式。
- 火山方舟风格的异步任务字段：`model`、`content[]`、`duration`、`resolution`、`ratio`、`generate_audio`、`watermark` 等。
- 文本、图片、视频、音频引用均使用公开 URL；也可以先通过素材接口得到 `asset://<asset_id>`，再放入 `content`。
- 轮询状态 `queued`、`running`、`succeeded`、`failed`、`expired`、`cancelled` 已映射到 New API 任务状态。
- 成功响应中的 `content.video_url` 会作为任务视频制品保存，并通过网关制品地址返回，避免把短期上游 URL 当作长期链接。

素材管理页面入口暂时隐藏，但后台素材代理已经复用同一套接口：选择 `runyuan` 渠道时，网关会将素材组、素材、真人认证请求转换为润元的 HMAC-SHA256 `Action` 协议；选择 `mobilecloud` 时仍使用移动云签名协议。这样可以先稳定视频链路，后续再开放网页素材库入口。

## 渠道配置

在管理后台新建 **任务插件** 渠道：

| 配置项 | 值 |
| --- | --- |
| 任务插件 key | `runyuan` |
| 基础 URL | `https://runy.yitd.cn`（或老板提供的私有地址） |
| API 密钥 | 润元 Bearer API Key |
| 模型 | `doubao-seedance-2.0` |

基础 URL 只填写域名，不要追加 `/v1` 或具体接口路径。插件会向上游发送：

```http
POST /v1/video/tasks
Authorization: Bearer <API_KEY>
Content-Type: application/json
```

渠道测试使用只读的 `GET /v1/video/tasks/channel-test-nonexistent` 预检，不会创建视频任务。该哨兵任务预期返回 404；网关将这个已接受的错误状态视为凭证和路由检查通过，不会把它误报为渠道故障。

## 对外调用示例

客户端只需要把 Base URL 换成 New API 地址，模型仍使用火山方舟兼容名称：

```bash
curl https://<NEW_API_HOST>/v1/videos \
  -H 'Authorization: Bearer <NEW_API_TOKEN>' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "doubao-seedance-2.0",
    "content": [{"type": "text", "text": "一只猫在海边奔跑，电影感，夕阳"}],
    "duration": 5,
    "resolution": "720p",
    "ratio": "16:9"
  }'
```

同一个公开模型可以同时配置多个渠道。New API 根据渠道权重、可用状态和密钥池选择实际的火山、移动云或润元上游；客户端不需要为每个厂商维护一套 SDK。

### 可选的后台素材代理

素材页面暂时不在侧边栏展示，但管理员仍可通过 `/api/mobilecloud/*` 管理接口操作。素材凭证放在**所选任务插件渠道**的高级配置中：

```json
{
  "task_plugin_key": "runyuan",
  "asset_enabled": true,
  "asset_base_url": "https://runy.yitd.cn",
  "asset_access_key": "ACCESS_KEY",
  "asset_secret_key": "SECRET_KEY",
  "asset_resource_pool": "CIDC-CORE-00"
}
```

`runyuan` 的 `asset_access_key`/`asset_secret_key` 用于 HMAC-SHA256 素材接口，视频任务本身仍使用渠道的 Bearer API Key。`asset_base_url` 留空时按插件默认地址填充；素材上传的文件先写入网关配置的本地或 S3 兼容对象存储，再把公开 URL 注册到上游。

## 故障定位

任务日志中应优先记录以下字段：

1. New API `task_id`；
2. 请求 `request_id` 与请求路径；
3. 用户、渠道、任务插件及插件版本；
4. 上游任务 ID；
5. 上游失败原因和最终状态。

管理员任务日志列表已显示失败原因、请求 ID 和上游任务 ID，详情弹窗还会显示实际模型、插件版本及节点信息。出现失败时可按请求 ID 在通用使用日志中交叉查询，再根据上游任务 ID 到对应厂商控制台定位。

## 上线前检查

- 确认润元 API Key、模型白名单和服务器出口 IP 白名单。
- 先用哨兵任务查询完成渠道测试，再发送一条最小 5 秒文生视频任务。
- 检查任务轮询、失败状态、视频制品下载和费用结算；删除/取消应验证网关本地状态转换。
- 为同一模型配置移动云与润元两个渠道，设置权重和自动禁用策略，验证故障切换。
- 生产环境设置 `TASK_PUBLIC_ADDRESS`，确保制品地址可被客户端访问；若启用网关代传素材，`ServerAddress`（或反向代理地址）也必须能被上游访问。
