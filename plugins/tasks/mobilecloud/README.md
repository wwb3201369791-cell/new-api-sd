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
- Volcano Ark-compatible `POST /api/v3/contents/generations/tasks` and
  `GET /api/v3/contents/generations/tasks/:task_id` (the same JSON body and
  task status shape used by Seedance clients)
- `POST /v1/responses` and retrieval through the Responses protocol
- Native task routes under `/mobilecloud/api/v3/contents/generations/tasks`

For the Ark-compatible path, clients send the normal `Authorization: Bearer
<new-api-token>` header to this gateway. New API authenticates that token and
uses the configured Mobile Cloud channel key for the upstream Bearer request;
the upstream temporary media URLs are replaced with gateway artifact URLs in
the task response.

Image, video, and audio references must be publicly reachable URLs. Multipart
file uploads are rejected until an asset/object-storage service is added.
The gateway stores the upstream response privately and returns a stable
`/v1/tasks/{task_id}/artifacts/video/content` capability URL for successful
Ark-compatible queries, so Mobile Cloud's short-lived signed URL is not
exposed as the long-term client URL. Configure `TASK_PUBLIC_ADDRESS` (or the
existing server address setting) before enabling this response path.
