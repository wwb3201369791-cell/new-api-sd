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

The supported public model alias is `doubao-seedance-2.0`. A channel
`model_mapping` may map that alias to a deployment-specific upstream model.

The administrator channel-test action uses the provider's non-billing
`/api/v3/mapping/query` endpoint through `buildChannelTestRequest`; it never
submits a video generation task.

## Supported host protocols

- `POST /v1/videos` and `GET /v1/videos/:task_id`
- `POST /v1/responses` and retrieval through the Responses protocol
- Native task routes under `/mobilecloud/api/v3/contents/generations/tasks`

Image, video, and audio references must be publicly reachable URLs. Multipart
file uploads are rejected until an asset/object-storage service is added.
