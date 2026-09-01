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

## 渠道配置

在管理后台新增一个“任务插件”渠道：

1. 任务插件选择 `Mobile Cloud Seedance`，模型选择 `doubao-seedance-2.0`。
2. 基础 URL 填移动云模型服务区域地址，例如 `https://zhenze-huhehaote.cmecloud.cn`，不要追加 `/api/v3`。
3. API 密钥填写移动云模型服务的 `MAAS_API_KEY`（Bearer key）。
4. 如需素材库，在渠道高级设置的 JSON 中增加：

```json
{
  "task_plugin_key": "mobilecloud",
  "asset_base_url": "https://ecloud.10086.cn",
  "asset_access_key": "ACCESS_KEY",
  "asset_secret_key": "SECRET_KEY",
  "asset_resource_pool": "CIDC-CORE-00"
}
```

素材 AK/SK 与视频生成 Bearer key 分开保存，服务端不会返回给用户。

## 素材管理 API

用户令牌调用以下网关接口即可管理素材组和素材：

- `GET|POST /api/mobilecloud/asset-groups`
- `GET|PUT|DELETE /api/mobilecloud/asset-groups/:group_id`
- `GET|POST /api/mobilecloud/assets`
- `GET|PUT|DELETE /api/mobilecloud/assets/:asset_id`
- `POST /api/mobilecloud/real-person-auth/sessions`
- `POST /api/mobilecloud/real-person-auth/asset-group/by-byted-token`

网关按移动云 V2.0 规则签名（HMAC-SHA1，支持 HMAC-SHA256），并保留一份
脱敏后的本地索引。创建素材使用移动云规定的公网 HTTP(S) URL，移动云会将
对象下载到其 EOS 存储并返回约 12 小时有效的预签名 URL；因此当前不要求在
网关服务器上落地大文件，也不会占用服务器磁盘。真人素材组必须经过官方
活体认证流程，网关只转发认证会话和 Token 查询，不绕过认证。

## 润元扩展

对外协议、任务模型、素材 API 的网关边界已经独立。接入润元时新增一个
provider 插件，复用列表/取消/删除/素材控制器，只实现其鉴权、请求字段、
状态映射和结果 URL 适配即可；客户端 URL 不变。

## 验收

没有真实移动云 AK/SK 和可访问素材 URL 时只能完成协议与签名单元测试，不能
替代真实计费任务的端到端验收。上线前应使用测试账户验证：创建任务、轮询成功、
视频代理、取消退款、创建素材、查询素材和预签名 URL 访问。
