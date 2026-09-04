/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

export type DocsAudience = 'public' | 'user' | 'admin'

export type DocsPageId =
  | 'overview'
  | 'guide/user'
  | 'guide/admin'
  | 'guide/seedance'
  | 'api'

export type DocsPage = {
  id: DocsPageId
  title: string
  summary: string
  audience: DocsAudience
  markdown: string
}

export type DocsNavItem = {
  id: DocsPageId
  title: string
  href: string
}

export type DocsNavGroup = {
  title: string
  items: DocsNavItem[]
}

const page = (
  id: DocsPageId,
  title: string,
  summary: string,
  audience: DocsAudience,
  markdown: string
): DocsPage => ({ id, title, summary, audience, markdown })

export const DOCS_PAGES: Record<DocsPageId, DocsPage> = {
  overview: page(
    'overview',
    'Overview',
    'Understand the gateway, channels, and the two client-facing contracts.',
    'public',
    `# New API gateway

New API is the client-facing gateway between your application and configured model channels. Clients keep one API base URL and one New API token; administrators decide which channel serves each model.

## What the gateway does

- Presents an OpenAI-compatible \`/v1\` surface for chat, media, and task models.
- Selects channels by model, group, priority, weight, health, and retry policy.
- Keeps channel credentials on the server and maps channel-specific fields, status codes, and task states.
- Correlates the client request ID, upstream request ID, and asynchronous task ID in logs.

## Seedance support

The Seedance task plugin accepts the same task request regardless of which compatible channel is selected. A client does not need a separate SDK for each channel.

## Choose a guide

- **Users:** obtain a token, call \`/v1/video/generations\`, poll the task, and register public asset URLs.
- **Administrators:** configure channels, provider credentials, asset-library access, routing, and diagnostics.
- **API reference:** copy request/response examples for the stable public contract.
`
  ),
  'guide/user': page(
    'guide/user',
    'User guide',
    'Call models through one New API endpoint without knowing the selected provider.',
    'user',
    `# User guide

## 1. Create and protect a token

Create an API key in the console and send it as \`Authorization: Bearer TOKEN\`. Treat it as a password. Do not put it in browser bundles, public repositories, or screenshots.

## 2. Use the stable base URL

\`\`\`text
https://YOUR_HOST/v1
\`\`\`

The gateway chooses the configured channel. Clients should use the model name shown in the model list, not a channel-specific hostname.

## 3. Submit and poll a Seedance task

Submit a video task to \`POST /v1/video/generations\` (the legacy-compatible path) or \`POST /v1/videos\` (the OpenAI video path). The response contains a task ID. Poll the matching retrieve endpoint until the task is \`succeeded\` or \`failed\`. Keep the returned request ID when opening a support ticket.

## 4. Use the asset API

The preferred ToB flow is a provider-reachable public HTTPS URL:

\`\`\`http
POST /v1/assets
Authorization: Bearer TOKEN
Content-Type: application/json

{"assetName":"character","assetType":"Image","assetUrl":"https://PUBLIC_HOST/character.png"}
\`\`\`

Every user receives a default asset group. Omit \`groupId\` to use it, or create a group with \`POST /v1/asset-groups\` and pass the returned ID when separation is needed. The gateway enforces ownership so a user cannot operate another user's group.

The provider downloads the URL asynchronously; the gateway does not need to retain the media file in the ToB URL flow.
`
  ),
  'guide/admin': page(
    'guide/admin',
    'Administrator guide',
    'Configure channels, provider credentials, routing, and operational diagnostics.',
    'admin',
    `# Administrator guide

## Channel setup

Create one channel per upstream credential set and expose the same model name, for example \`doubao-seedance-2.0\`. Keep the video API key in the channel credential. If Mobile Cloud asset management is enabled, enter its separate Access Key and Secret Key in channel advanced settings.

## Asset-library policy

The asset switch is optional. When enabled, the gateway signs asset requests for the selected provider and keeps credentials server-side. A default per-user group is created lazily; explicit group creation is available through \`POST /v1/asset-groups\`. Provider group IDs are never accepted as a substitute for the local ownership check.

## Routing and errors

Use model/group routing to balance Mobile Cloud and RunYuan. The gateway maps upstream failures to stable client errors: authentication failures, rate limits, timeouts, unavailable providers, and validation errors are recorded with the upstream response details while the client receives a readable message.

## Logs and troubleshooting

Search by the New API request ID, then inspect the upstream request ID and task ID. Sensitive headers and credential values are redacted. For a failed asset request, verify the provider endpoint, resource pool, Access Key/Secret Key pair, public URL reachability, DNS/TLS, and provider-side allowlists.
`
  ),
  'guide/seedance': page(
    'guide/seedance',
    'Seedance / SD guide',
    'Use one public contract for all configured task channels.',
    'public',
    `# Seedance / SD guide

## Client contract

Use the New API base URL and token for every request. The client does not select a task channel directly; channel routing is an administrator concern.

## Video request

\`\`\`json
{
  "model": "doubao-seedance-2.0",
  "prompt": "A short product introduction",
  "duration": 5,
  "aspect_ratio": "16:9"
}
\`\`\`

The gateway normalizes provider fields and returns a task object. Poll at the documented interval, honor \`Retry-After\` when present, and stop polling after the configured timeout.

## Asset references

Use the asset ID returned by \`POST /v1/assets\` as \`asset://ASSET_ID\`, or pass an approved public URL when the selected channel allows it. Classify the media as \`Image\`, \`Video\`, or \`Audio\`.

## Virtual and real-person assets

- **Virtual/cartoon characters:** register the image as \`assetType: "Image"\` in the default AIGC group (or a group created with \`POST /v1/asset-groups\`). No liveness session is required.
- **Real people:** use \`POST /v1/real-person-auth/sessions\` first, let the person complete the returned verification page, then exchange the returned token with \`POST /v1/real-person-auth/asset-group/by-byted-token\`. Use the returned group ID for the verified asset. Do not submit a real-person image to the ordinary AIGC group flow.
- **Audio:** register it with \`assetType: "Audio"\` and use the returned \`asset://ASSET_ID\` reference where the selected video model accepts audio.

The gateway returns semantic review messages only. Channel names, raw moderation codes, signed URLs, and credential details are available to administrators in redacted diagnostics, not to API users.

## Debug checklist

Record \`X-Oneapi-Request-Id\`, the task ID, and the final status. A 4xx usually means request or credential validation; a timeout or 5xx means the gateway could not obtain a usable upstream result. Use the administrator logs to distinguish those cases.
`
  ),
  api: page(
    'api',
    'API reference',
    'Stable endpoints and authentication examples for client integrations.',
    'public',
    `# API reference

## Authentication

\`\`\`http
Authorization: Bearer TOKEN
Content-Type: application/json
X-Request-Id: OPTIONAL_CLIENT_ID
\`\`\`

The gateway returns \`X-Oneapi-Request-Id\`. Save it with the response body for support and audit trails.

## Core endpoints

| Method | Path | Purpose |
| --- | --- | --- |
| GET | \`/v1/models\` | List available chat and task-plugin models |
| POST | \`/v1/video/generations\` | Submit a legacy-compatible Seedance task |
| POST | \`/v1/videos\` | Submit an OpenAI video task |
| GET | \`/v1/video/generations/{taskId}\` | Poll a legacy-compatible task |
| GET | \`/v1/videos/{taskId}\` | Poll an OpenAI video task |
| GET | \`/api/v3/contents/generations/tasks\` | Ark-compatible task listing |
| GET | \`/v1/asset-groups\` | List groups visible to the current user |
| POST | \`/v1/asset-groups\` | Create a uniquely named group |
| POST | \`/v1/assets\` | Register an approved public URL |
| GET | \`/v1/assets\` | List assets in an owned group |
| GET | \`/v1/assets/{assetId}\` | Read an asset detail |
| POST | \`/v1/real-person-auth/sessions\` | Start a real-person verification session |
| POST | \`/v1/real-person-auth/asset-group/by-byted-token\` | Resolve the verified real-person group |

## Idempotency and retries

Send an idempotency key for task creation when the client may retry. Retry only transient failures and preserve the original request ID. Do not retry a rejected URL, invalid credential, or malformed JSON without fixing the request.
`
  ),
}

export const DOCS_NAV_GROUPS: DocsNavGroup[] = [
  {
    title: 'Introduction',
    items: [{ id: 'overview', title: 'Overview', href: '/docs' }],
  },
  {
    title: 'Guides',
    items: [
      { id: 'guide/user', title: 'User guide', href: '/docs/guide/user' },
      {
        id: 'guide/admin',
        title: 'Administrator guide',
        href: '/docs/guide/admin',
      },
      {
        id: 'guide/seedance',
        title: 'Seedance / SD guide',
        href: '/docs/guide/seedance',
      },
    ],
  },
  {
    title: 'Reference',
    items: [{ id: 'api', title: 'API reference', href: '/docs/api' }],
  },
]

/**
 * Chinese documentation is kept beside the English source instead of adding
 * another locale or another language switch. The existing application locale
 * decides which copy is rendered in the documentation center.
 */
const DOCS_MARKDOWN_ZH: Partial<Record<DocsPageId, string>> = {
  overview: `# New API 网关

New API 是位于客户应用与模型服务之间的统一网关。客户只需要一个 API 地址和一枚 New API 密钥，管理员负责决定每个模型实际使用的渠道。

## 网关提供的能力

- 通过兼容 OpenAI 的 \`/v1\` 接口提供对话、媒体和任务模型。
- 按模型、分组、优先级、权重、健康状态和重试策略选择渠道。
- 将渠道凭证保存在服务端，并完成字段、状态码和任务状态映射。
- 在日志中关联客户请求 ID、上游请求 ID 和异步任务 ID。

## Seedance 支持

Seedance 任务插件使用统一的任务请求格式。实际渠道由管理员配置，客户不需要为每个渠道单独开发 SDK。

## 选择文档

- **用户：** 获取密钥、调用 \`/v1/video/generations\`、轮询任务并登记公网素材地址。
- **管理员：** 配置渠道、供应商凭证、素材库、路由和诊断信息。
- **API 参考：** 查看稳定的请求与响应示例。
`,
  'guide/user': `# 用户指南

## 1. 创建并保护密钥

在控制台创建 API 密钥，并使用 \`Authorization: Bearer TOKEN\` 请求。密钥等同于密码，不要放进浏览器代码、公开仓库或截图。

## 2. 使用统一地址

\`\`\`text
https://YOUR_HOST/v1
\`\`\`

网关会根据模型和管理员路由配置选择渠道。客户端只使用模型列表中的模型名，不需要知道渠道的主机地址。

## 3. 提交并轮询 Seedance 任务

视频任务可调用 \`POST /v1/video/generations\`（兼容旧路径）或 \`POST /v1/videos\`（OpenAI 视频路径）。响应会返回任务 ID，使用对应的查询接口轮询，直到状态为 \`succeeded\` 或 \`failed\`。遇到问题时请保留响应中的请求 ID。

## 4. 使用素材接口

ToB 场景优先使用供应商可访问的公网 HTTPS 地址：

\`\`\`http
POST /v1/assets
Authorization: Bearer TOKEN
Content-Type: application/json

{"assetName":"character","assetType":"Image","assetUrl":"https://PUBLIC_HOST/character.png"}
\`\`\`

每个用户都有默认素材组。不传 \`groupId\` 时自动使用默认组；需要隔离时调用 \`POST /v1/asset-groups\` 创建素材组，并使用返回的 ID。网关会校验归属，用户不能操作其他用户的素材组。

素材由上游异步下载，公网 URL 模式下网关不保存媒体文件。
`,
  'guide/admin': `# 管理员指南

## 渠道配置

每套上游凭证创建一个渠道，并可以为多个渠道配置相同的模型名，例如 \`doubao-seedance-2.0\`。视频生成使用渠道凭证；移动云素材管理启用后，在渠道高级设置中填写独立的 Access Key 和 Secret Key。

## 素材库策略

素材库开关是可选的。启用后，网关为选定供应商签名素材请求，并将凭证保存在服务端。每个用户的默认素材组按需创建，也可以通过 \`POST /v1/asset-groups\` 显式创建其他分组。上游组 ID 不能绕过本地归属校验。

## 路由与错误

通过模型和分组路由在移动云、润元之间分配流量。网关把鉴权失败、限流、超时、上游不可用和参数错误映射为稳定的客户端错误，同时在日志中保留脱敏后的上游详情。

## 日志与排查

先按 New API 请求 ID 搜索，再查看上游请求 ID 和任务 ID。敏感请求头与凭证会脱敏。素材请求失败时，检查供应商端点、资源池、Access Key/Secret Key、公网 URL 的可达性、DNS/TLS 以及供应商侧访问策略。
`,
  'guide/seedance': `# Seedance / SD 使用指南

## 客户端契约

所有请求都使用 New API 地址和密钥。客户端不直接选择具体渠道，渠道选择由管理员的路由配置完成。

## 视频请求

\`\`\`json
{
  "model": "doubao-seedance-2.0",
  "prompt": "A short product introduction",
  "duration": 5,
  "aspect_ratio": "16:9"
}
\`\`\`

网关会完成供应商字段映射并返回任务对象。按文档间隔轮询，存在 \`Retry-After\` 时遵循该值，并在配置的超时后停止轮询。

## 素材引用

将 \`POST /v1/assets\` 返回的素材 ID 写成 \`asset://ASSET_ID\` 使用；选定渠道允许时也可以直接传入已审核的公网 URL。素材类型填写 \`Image\`、\`Video\` 或 \`Audio\`。

## 虚拟人物与真实人物

- **虚拟人物/卡通人物：** 使用默认 AIGC 素材组，或先调用 \`POST /v1/asset-groups\` 创建分组，再以 \`assetType: "Image"\` 登记素材。不需要真人认证会话。
- **真实人物：** 先调用 \`POST /v1/real-person-auth/sessions\` 创建认证会话，让本人完成返回的认证页面，再将返回的令牌提交到 \`POST /v1/real-person-auth/asset-group/by-byted-token\` 获取已认证素材组，随后使用该组 ID 登记素材。不要把真实人物图片直接放入普通 AIGC 流程。
- **音频：** 以 \`assetType: "Audio"\` 登记，在视频模型支持时引用返回的 \`asset://ASSET_ID\`。

网关只向客户返回语义化的审核提示；渠道名称、原始审核码、签名 URL 和凭证详情仅保留在管理员可见的脱敏诊断中。

## 排查清单

记录 \`X-Oneapi-Request-Id\`、任务 ID 和最终状态。4xx 通常表示请求或凭证校验失败；超时或 5xx 表示网关未能及时取得可用的上游结果。管理员可在日志中区分两类问题。
`,
  api: `# API 参考

## 鉴权

\`\`\`http
Authorization: Bearer TOKEN
Content-Type: application/json
X-Request-Id: OPTIONAL_CLIENT_ID
\`\`\`

网关会返回 \`X-Oneapi-Request-Id\`。请将它与响应体一起保存，用于审计和问题定位。

## 核心接口

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| GET | \`/v1/models\` | 列出可用的对话和任务插件模型 |
| POST | \`/v1/video/generations\` | 提交兼容旧路径的 Seedance 任务 |
| POST | \`/v1/videos\` | 提交 OpenAI 视频任务 |
| GET | \`/v1/video/generations/{taskId}\` | 查询旧路径任务 |
| GET | \`/v1/videos/{taskId}\` | 查询 OpenAI 视频任务 |
| GET | \`/api/v3/contents/generations/tasks\` | Ark 兼容任务列表 |
| GET | \`/v1/asset-groups\` | 查询当前用户可见的素材组 |
| POST | \`/v1/asset-groups\` | 创建名称唯一的素材组 |
| POST | \`/v1/assets\` | 登记已审核且可访问的公网地址 |
| GET | \`/v1/assets\` | 查询所属素材组中的素材 |
| GET | \`/v1/assets/{assetId}\` | 查询素材详情 |
| POST | \`/v1/real-person-auth/sessions\` | 创建真人认证会话 |
| POST | \`/v1/real-person-auth/asset-group/by-byted-token\` | 获取已认证真人素材组 |

## 幂等与重试

任务创建可能重试时请携带幂等键，并保留原始请求 ID。只重试临时性失败；拒绝的 URL、无效凭证或格式错误应先修正请求。
`,
}

export function getDocsMarkdown(page: DocsPage, language?: string): string {
  if (language?.toLowerCase().startsWith('zh')) {
    return DOCS_MARKDOWN_ZH[page.id] ?? page.markdown
  }
  return page.markdown
}

export function getDocsPage(id: string | undefined): DocsPage {
  if (!id) return DOCS_PAGES.overview
  return DOCS_PAGES[id as DocsPageId] ?? DOCS_PAGES.overview
}

export function getDocsPageId(id: string | undefined): DocsPageId {
  return getDocsPage(id).id
}

export function getDocsNavItem(id: DocsPageId): DocsNavItem {
  return (
    DOCS_NAV_GROUPS.flatMap((group) => group.items).find(
      (item) => item.id === id
    ) ?? { id: 'overview', title: 'Overview', href: '/docs' }
  )
}

export function getDocsHeadings(markdown: string): string[] {
  return markdown
    .split('\n')
    .map((line) => /^#{2,3}\s+(.+)$/.exec(line)?.[1]?.trim())
    .filter((heading): heading is string => Boolean(heading))
}

export function slugifyDocsHeading(heading: string): string {
  const slug = heading
    .replaceAll('`', '')
    .toLowerCase()
    .replaceAll(/[^\p{L}\p{N}]+/gu, '-')
    .replaceAll(/^-+|-+$/g, '')

  return slug || 'section'
}
