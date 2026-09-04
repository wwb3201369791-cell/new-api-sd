# Runyuan Seedance task plugin

This plugin keeps the public Volcano Ark/OpenAI-compatible video contract while
forwarding asynchronous generation tasks to Runyuan's `/v1/video/tasks` API.

Configure a Task Plugin channel with `task_plugin_key=runyuan`, base URL
`https://runy.yitd.cn`, the Runyuan Bearer API key, and model
`doubao-seedance-2.0`. The plugin handles task creation, polling, local task
lifecycle responses, artifact forwarding, Responses events, and
completion-token usage extraction.

The Runyuan asset APIs use a separate Volcano-style AK/SK HMAC-SHA256 protocol.
The backend asset proxy converts the shared `/api/mobilecloud/*` management
contract to that protocol when a `runyuan` channel is selected. The asset UI is
currently hidden while the video path is validated end-to-end; operators can
still use the backend endpoints with `asset_enabled=true` and the asset AK/SK
fields in the channel settings.

Read-only asset actions are retried with a freshly generated signature when the
upstream closes the connection before returning an HTTP response. Mutating
actions are deliberately not replayed. If Runyuan returns one account-level
AIGC group for multiple customers sharing an AK/SK pair, New API applies a
second local asset-ID ownership filter so customer data is not exposed across
tenant boundaries.
