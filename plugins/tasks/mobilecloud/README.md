# Mobile Cloud Seedance task plugin

This plugin drives the China Mobile Cloud Seedance 2.0 non-encrypted REST API.

## Channel setup

Create a **Task Plugin** channel and select `mobilecloud` as `task_plugin_key`.
Use the gateway host only as the Base URL, for example:

```text
https://zhenze-huhehaote.cmecloud.cn
```

The plugin appends `/api/v3/contents/generations/tasks` for submission and
query requests. Store the Mobile Cloud MAAS API key in the channel key field;
the plugin sends it as `Authorization: Bearer <key>`.

The supported Mobile Cloud request alias is `doubao-seedance-2.0`; the
provider may return the concrete name `doubao-seedance-2-0-260128` in task
status data. To let a Volcano Ark client keep sending the concrete model name,
configure the channel's public model list and mapping as follows (the mapping
value is the Mobile Cloud alias):

```json
{
  "models": ["doubao-seedance-2-0-260128"],
  "model_mapping": {
    "doubao-seedance-2-0-260128": "doubao-seedance-2.0"
  }
}
```

The same mapping mechanism can expose any customer-facing model name without
changing the upstream request shape.

The administrator channel-test action uses the provider's non-billing
`/api/v3/mapping/query` endpoint through `buildChannelTestRequest`; it never
submits a video generation task.

## Supported host protocols

- `POST /v1/videos` and `GET /v1/videos/:task_id`
- `GET /v1/videos` and `DELETE /v1/videos/:task_id` (list/cancel/delete)
- Volcano Ark-compatible `POST /api/v3/contents/generations/tasks` and
  `GET /api/v3/contents/generations/tasks/:task_id` (the same JSON body and
  task status shape used by Seedance clients)
- Ark-compatible list and DELETE lifecycle endpoints are also exposed.
- `POST /v1/responses` and retrieval through the Responses protocol
- Native task routes under `/mobilecloud/api/v3/contents/generations/tasks`

For the Ark-compatible path, clients send the normal `Authorization: Bearer
<new-api-token>` header to this gateway. New API authenticates that token and
uses the configured Mobile Cloud channel key for the upstream Bearer request;
the upstream temporary media URLs are replaced with gateway artifact URLs in
the task response.

Image, video, and audio references can be public URLs or `asset://{asset_id}`
references created through the Mobile Cloud asset API. Multipart binary input
is intentionally not uploaded to the gateway: Mobile Cloud's official asset
API accepts a public URL and stores the object in its EOS storage.
The gateway stores the upstream response privately and returns a stable
`/v1/tasks/{task_id}/artifacts/video/content` capability URL for successful
Ark-compatible queries, so Mobile Cloud's short-lived signed URL is not
exposed as the long-term client URL. Configure `TASK_PUBLIC_ADDRESS` (or the
existing server address setting) before enabling this response path.

## Asset management

Enable **Mobile Cloud asset library** in the channel's Advanced settings and
enter the separate `asset_access_key` and `asset_secret_key` credentials.
Authenticated user APIs under `/api/mobilecloud/asset-groups` and
`/api/mobilecloud/assets` then proxy the official signed OpenAPI. The web
Asset Library page also supports local multipart upload through
`POST /api/mobilecloud/uploads`; the gateway stores the bytes in local disk or
an S3-compatible backend and registers the resulting public URL with the
selected Mobile Cloud group. Configure `ASSET_STORAGE_PUBLIC_URL` (or the
gateway's public address) so the upstream can fetch local objects. The
gateway also supports the real-person verification session/token flow and
the upstream usage/deduction query/export endpoints under
`/api/mobilecloud/billing`. Existing channels without `asset_enabled` keep
the legacy behavior when both asset credentials are present; an explicit
`false` disables the library while retaining the credentials. See
`docs/mobilecloud-seedance.md` for request fields and the rollout checklist.
