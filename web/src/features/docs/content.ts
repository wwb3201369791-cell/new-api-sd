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
  | 'installation'
  | 'guide/user'
  | 'guide/admin'
  | 'guide/seedance'
  | 'api'
  | 'support'
  | 'policy'

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

New API is the client-facing gateway between your application and configured model providers. Clients keep one API base URL and one New API token; administrators decide which channel serves each model.

## What the gateway does

- Presents an OpenAI-compatible \`/v1\` surface for chat, media, and task models.
- Selects channels by model, group, priority, weight, health, and retry policy.
- Keeps provider credentials on the server and maps provider-specific fields, status codes, and task states.
- Correlates the client request ID, upstream request ID, and asynchronous task ID in logs.

## Seedance support

The Seedance task plugin accepts the same task request regardless of whether the selected channel is Mobile Cloud, RunYuan, or another compatible provider. A client does not need a separate SDK for every upstream.

## Choose a guide

- **Users:** obtain a token, call \`/v1/video/generations\`, poll the task, and register public asset URLs.
- **Administrators:** configure channels, provider credentials, asset-library access, routing, and diagnostics.
- **API reference:** copy request/response examples for the stable public contract.
`
  ),
  installation: page(
    'installation',
    'Installation',
    'Run the gateway locally or in a persistent production deployment.',
    'public',
    `# Installation

## Docker Compose (recommended)

Persist the database, uploads, and application configuration on a data volume such as \`/data\`. Keep secrets in environment variables or the deployment secret store, not in the image or documentation.

\`\`\`bash
docker compose up -d
docker compose logs -f new-api
\`\`\`

## First-run checklist

1. Complete the setup wizard and create the administrator account.
2. Configure a public HTTPS hostname for client traffic and callback URLs.
3. Create a model group and add a provider channel.
4. Create a test API token and send a request through \`/v1\`.
5. Enable backups and monitor request/task logs before onboarding users.

The documentation link can remain local at \`/docs\`, or an administrator can configure a complete external URL when a separately hosted documentation site is preferred.
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

The gateway chooses the configured channel. Clients should use the model name shown in the model list, not a provider-specific hostname.

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
    'Use one public contract for Mobile Cloud and RunYuan task channels.',
    'public',
    `# Seedance / SD guide

## Client contract

Use the New API base URL and token for every request. The client does not select Mobile Cloud or RunYuan directly; channel routing is an administrator concern.

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

Use the asset ID returned by \`POST /v1/assets\` as \`asset://ASSET_ID\`, or pass a provider-approved public URL when the upstream contract allows it. Classify the media as \`Image\`, \`Video\`, or \`Audio\`; virtual-person, real-person, and audio policies remain provider-side validation rules.

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
| POST | \`/v1/assets\` | Register a provider-reachable public URL |
| GET | \`/v1/assets\` | List assets in an owned group |
| GET | \`/v1/assets/{assetId}\` | Read an asset detail |

## Idempotency and retries

Send an idempotency key for task creation when the client may retry. Retry only transient failures and preserve the original request ID. Do not retry a rejected URL, invalid credential, or malformed JSON without fixing the request.
`
  ),
  support: page(
    'support',
    'Help and support',
    'Collect the identifiers and checks needed to resolve a failed request quickly.',
    'public',
    `# Help and support

Include these fields in a support ticket:

- timestamp with timezone;
- New API request ID from the response header;
- upstream request ID and task ID, if returned;
- endpoint, model, and a redacted request body;
- client-visible status and message.

Never send an API token, provider secret, signed URL query, or private media URL in a ticket. Administrators can inspect the redacted request/response pair and upstream error code in the operation logs.

For asset failures, first verify that the URL is reachable from the provider network and returns the expected content type. A URL that opens in a local browser may still be blocked by provider egress policy.
`
  ),
  policy: page(
    'policy',
    'Compliance and use policy',
    'Operate the gateway only with authorized accounts, content, and provider capabilities.',
    'public',
    `# Compliance and use policy

Use only provider accounts, API keys, quotas, media, and model capabilities for which your organization has authorization. Follow upstream terms, applicable laws, privacy requirements, retention rules, and content-safety policies.

Do not place secrets in client-side code or documentation. Limit administrator access, rotate provider credentials, retain only the logs required for operations, and remove test assets after verification.

This page is an operational reminder rather than legal advice. Consult your compliance owner for requirements specific to your deployment and customer contracts.
`
  ),
}

export const DOCS_NAV_GROUPS: DocsNavGroup[] = [
  {
    title: 'Introduction',
    items: [
      { id: 'overview', title: 'Overview', href: '/docs' },
      { id: 'installation', title: 'Installation', href: '/docs/installation' },
    ],
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
    items: [
      { id: 'api', title: 'API reference', href: '/docs/api' },
      { id: 'support', title: 'Help and support', href: '/docs/support' },
      {
        id: 'policy',
        title: 'Compliance and use policy',
        href: '/docs/policy',
      },
    ],
  },
]

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
