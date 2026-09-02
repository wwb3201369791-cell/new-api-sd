export const meta = {
  apiVersion: 1,
  key: "runyuan",
  name: "Runyuan Seedance",
  icon: "text:MC",
  description: {
    en: "China Runyuan Seedance video generation (text-to-video, image-to-video, and multimodal input)",
    zh: "润元 Seedance 视频生成（文生视频、图生视频和多模态输入）",
  },
  version: "1.0.4",
  author: { name: "QuantumNous" },
  // Third-party task plugin channels are bound by task_plugin_key. Keep the
  // public alias documented by Runyuan; channel model_mapping can map it
  // to a deployment-specific model name when required.
  // Multiple task-provider plugins may expose this same public model. The
  // host chooses the configured channel by task_plugin_key.
  sharedModels: true,
  models: ["doubao-seedance-2.0", "doubao-seedance-2-0-260128"],
  fetchMode: "per_task",
  usageSchema: {
    tokens: {
      type: "number",
      unit: "token",
      description: {
        en: "Upstream billing tokens (estimated at submit, actual on completion).",
        zh: "上游计费 token（提交时预估，完成后按实际值）。",
      },
    },
    resolution: {
      enum: ["480p", "720p", "1080p"],
      description: {
        en: "Output video resolution; Seedance token unit price varies by resolution tier.",
        zh: "输出视频分辨率；Seedance token 单价随分辨率档位变化。",
      },
    },
    video_input: {
      enum: ["none", "video"],
      description: {
        en: "Whether the request includes reference video input; Seedance prices video-to-video tokens at a lower unit rate.",
        zh: "请求是否包含参考视频输入；Seedance 对视频生视频 token 按更低单价计费。",
      },
    },
  },
  // Runyuan bills Seedance by completion_tokens. The submit estimate is
  // intentionally conservative; extractUsageOnComplete replaces it with the
  // usage.completion_tokens value returned by the provider.
  usageExamples: [
    { label: "480p · 5s", facts: { tokens: 48038, resolution: "480p", video_input: "none" } },
    { label: "720p · 5s", facts: { tokens: 108000, resolution: "720p", video_input: "none" } },
    { label: "1080p · 5s", facts: { tokens: 243000, resolution: "1080p", video_input: "none" } },
    { label: "720p · 10s", facts: { tokens: 216000, resolution: "720p", video_input: "none" } },
    { label: "720p · 5s (+4s 输入视频)", facts: { tokens: 194400, resolution: "720p", video_input: "video" } },
  ],
  routes: [
    { method: "POST", path: "/runyuan/v1/video/tasks", type: "submit", decode: "createTask", render: "taskCreated" },
    { method: "GET", path: "/runyuan/v1/video/tasks/:task_id", type: "query", render: "taskStatus" },
  ],
  protocols: [{ name: "openai_responses", supports: ["stream", "sync", "background"] }, "openai_video"],
};

function trimmed(value) {
  return String(value || "").trim();
}

function normalizeRunyuanModel(value) {
  const model = trimmed(value);
  return model === "doubao-seedance-2-0-260128" ? "doubao-seedance-2.0" : model;
}

function draftTaskIds(content) {
  const ids = [];
  if (!Array.isArray(content)) return ids;
  for (const item of content) {
    if (!item || typeof item !== "object" || Array.isArray(item)) continue;
    if (item.type !== "draft_task") continue;
    const draft = item.draft_task;
    if (!draft || typeof draft !== "object" || Array.isArray(draft)) continue;
    const id = trimmed(draft.id);
    if (id) ids.push(id);
  }
  return ids;
}

function rewriteDraftTaskContent(content, originTasks) {
  if (!Array.isArray(content)) return content;
  return content.map(function (item) {
    if (!item || typeof item !== "object" || Array.isArray(item) || item.type !== "draft_task") return item;
    const draft = item.draft_task;
    if (!draft || typeof draft !== "object" || Array.isArray(draft) || !trimmed(draft.id)) return item;
    const publicId = trimmed(draft.id);
    let upstream = "";
    if (Array.isArray(originTasks)) {
      for (const task of originTasks) {
        if (task && task.taskId === publicId) {
          upstream = trimmed(task.upstreamTaskId);
          break;
        }
      }
    }
    if (!upstream) throw new Error("origin task is unavailable");
    return Object.assign({}, item, { draft_task: Object.assign({}, draft, { id: upstream }) });
  });
}

function normalizeResolution(value) {
  const raw = trimmed(value).toLowerCase();
  if (["480p", "720p", "1080p"].includes(raw)) return raw;
  const parts = raw.replace("*", "x").split("x");
  if (parts.length !== 2) return "720p";
  const max = Math.max(Number(parts[0]), Number(parts[1]));
  if (max >= 1920) return "1080p";
  if (max >= 1280) return "720p";
  return "480p";
}

function hasVideo(content) {
  return Array.isArray(content) && content.some((item) => item && (item.type === "video_url" || Object.prototype.hasOwnProperty.call(item, "video_url")));
}

function contentFromRequest(req) {
  const metadata = req && req.metadata && typeof req.metadata === "object" && !Array.isArray(req.metadata) ? req.metadata : {};
  const content = [];
  if (Array.isArray(metadata.content)) content.push(...metadata.content);
  if (Array.isArray(req && req.content)) content.push(...req.content);
  return content;
}

function validateSeedanceContent(content) {
  if (!Array.isArray(content)) throw new Error("content must be an array");
  if (content.length === 0 || content.length > 5) throw new Error("content must contain between 1 and 5 items");
  for (const item of content) {
    if (!item || typeof item !== "object" || Array.isArray(item)) throw new Error("content items must be objects");
    const type = trimmed(item.type);
    if (!type) throw new Error("content item type is required");
    if (type === "text") {
      if (typeof item.text !== "string" || !trimmed(item.text)) throw new Error("text content requires a non-empty text field");
      continue;
    }
    if (["image_url", "video_url", "audio_url"].includes(type)) {
      const value = item[type];
      const url = typeof value === "string" ? value : value && typeof value === "object" ? value.url : "";
      if (!trimmed(url)) throw new Error(type + " content requires a public URL");
    }
  }
  return content;
}

function validateSeedanceDuration(value) {
  const seconds = Number(value);
  if (!Number.isInteger(seconds) || (seconds !== -1 && (seconds <= 0 || seconds > 3600))) {
    throw new Error("duration must be -1 or an integer between 1 and 3600");
  }
  return seconds;
}

// Max-pixel 16:9 dimensions per resolution tier. Used when ratio is absent or
// adaptive so the submit-time estimate overestimates rather than underestimates.
// Seedance estimate: seconds × width × height × 24 / 1024.
// Video input duration is omitted; extractUsageOnComplete overlays the real bill.
function resolutionMaxPixels(resolution) {
  if (resolution === "480p") return [854, 480];
  if (resolution === "1080p") return [1920, 1080];
  return [1280, 720];
}

function estimateTokens(seconds, resolution) {
  const dims = resolutionMaxPixels(resolution);
  return (seconds * dims[0] * dims[1] * 24) / 1024;
}

function videoInputRatio(model, resolution, content) {
  const video = hasVideo(content);
  const res = trimmed(resolution).toLowerCase();
  if (!video) return 1;
  // Runyuan pricing differentiates only by video input and resolution:
  // 56/92 for 480p/720p, and 62/102 for 1080p. The ratio is used when a
  // tiered billing expression models the provider's resource-package units.
  return res === "1080p" ? 62 / 102 : 56 / 92;
}

function responsesInput(req) {
  const texts = [],
    images = [];
  const input = req.input;
  if (typeof input === "string") texts.push(input);
  else if (Array.isArray(input)) {
    for (const item of input) {
      if (typeof item === "string") {
        texts.push(item);
        continue;
      }
      if (!item || typeof item !== "object" || Array.isArray(item)) continue;
      const content = item.content === undefined ? [item] : Array.isArray(item.content) ? item.content : [item.content];
      for (const part of content) {
        if (typeof part === "string") {
          texts.push(part);
          continue;
        }
        if (!part || typeof part !== "object" || Array.isArray(part)) continue;
        if (["input_text", "text"].includes(part.type) && typeof part.text === "string") texts.push(part.text);
        if (["input_image", "image_url"].includes(part.type)) {
          let image = part.image_url;
          if (image && typeof image === "object") image = image.url;
          if (trimmed(image)) images.push(trimmed(image));
        }
      }
    }
  }
  return {
    prompt: texts
      .filter(function (text) {
        return trimmed(text);
      })
      .join("\n"),
    images: images,
  };
}

function responsesVideoText(ctx) {
  const artifact = ctx && ctx.artifacts && ctx.artifacts.video;
  const url = trimmed(artifact && artifact.url);
  if (!url) throw new Error("video artifact is unavailable");
  const escaped = url.replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  return '<video controls src="' + escaped + '"></video>';
}

export const native = {
  createTask: function (ctx) {
    if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
    const body = ctx.body.value;
    if (!body || typeof body !== "object" || Array.isArray(body)) throw new Error("request body must be an object");
    const model = trimmed(body.model);
    if (!model) throw new Error("model is required");
    if (body.content !== undefined && !Array.isArray(body.content)) throw new Error("content must be an array");
    const content = Array.isArray(body.content) ? body.content : [];
    validateSeedanceContent(content);
    const texts = [];
    let hasReference = false;
    for (const item of content) {
      if (!item || typeof item !== "object" || Array.isArray(item)) continue;
      if (item.type === "text" && typeof item.text === "string") texts.push(item.text);
      else hasReference = true;
    }
    if (!texts.length && !hasReference) throw new Error("content is required");
    const requestBody = {
      model: model,
      prompt: texts
        .filter(function (text) {
          return trimmed(text);
        })
        .join("\n"),
      metadata: body,
    };
    if (Object.prototype.hasOwnProperty.call(body, "duration")) requestBody.seconds = validateSeedanceDuration(body.duration);
    const intent = { kind: "submit", model: model, action: hasReference ? "image_to_video" : "text_to_video", requestBody: requestBody };
    const originTaskIds = draftTaskIds(content);
    if (originTaskIds.length) intent.originTaskIds = originTaskIds;
    return intent;
  },
  taskCreated: function (ctx, task) {
    const data = task.data && typeof task.data === "object" && !Array.isArray(task.data) ? task.data : {};
    return Object.assign({}, data, { id: task.task_id });
  },
  taskStatus: function (ctx, task) {
    if (task.data && typeof task.data === "object" && !Array.isArray(task.data)) return Object.assign({}, task.data, { id: task.task_id });
    const statusMap = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "running", SUCCESS: "succeeded", FAILURE: "failed" };
    const output = { id: task.task_id, status: statusMap[task.status] || "queued" };
    if (task.fail_reason) output.error = { message: task.fail_reason };
    return output;
  },
  error: function (ctx, error) {
    return { error: { code: error.code, message: error.message } };
  },
};

export function buildSubmitRequest(ctx) {
  const req = ctx.requestBody;
  const baseUrl = trimmed(ctx && ctx.baseUrl).replace(/\/+$/, "");
  if (!baseUrl) throw new Error("baseUrl is required");
  const metadata = req.metadata && typeof req.metadata === "object" && !Array.isArray(req.metadata) ? req.metadata : {};
  // Responses requests are normalized into prompt/images/metadata, while the
  // Ark-compatible endpoint sends the provider's content[] shape directly.
  // Preserve both forms instead of dropping a direct content array.
  const body = Object.assign({}, metadata);
  for (const key of [
    "content",
    "duration",
    "ratio",
    "resolution",
    "watermark",
    "callback_url",
    "generate_audio",
    "return_last_frame",
    "seed",
    "camera_fixed",
    "frames",
    "tools",
    "safety_identifier",
    "service_tier",
    "execution_expires_after",
    "draft",
    "priority",
    "output_format",
  ]) {
    if (Object.prototype.hasOwnProperty.call(req, key)) body[key] = req[key];
  }
  body.model = req.model || "";
  const imageContent = [];
  const images = [];
  if (Array.isArray(req.images)) images.push(...req.images);
  for (const value of [req.image, req.input_reference]) {
    if (trimmed(value) && !images.includes(value)) images.push(value);
  }
  for (const url of images) {
    if (typeof url === "string" && trimmed(url)) imageContent.push({ type: "image_url", image_url: { url: trimmed(url) }, role: "reference_image" });
  }
  const directContent = Array.isArray(req.content) ? req.content.slice() : null;
  const metadataContent = contentFromRequest(req);
  const hasPrompt = trimmed(req.prompt) !== "";
  if (directContent) {
    body.content = directContent;
    // Add normalized image fields only when the direct content did not carry
    // the same URL already. This keeps the official content order intact.
    for (const item of imageContent) {
      const imageURL = item.image_url && item.image_url.url;
      const exists = body.content.some((candidate) => candidate && candidate.image_url && candidate.image_url.url === imageURL);
      if (!exists) body.content.push(item);
    }
  } else {
    body.content = imageContent.concat(metadataContent).filter(function (item) {
      return item && (!hasPrompt || item.type !== "text");
    });
    const hasReference = body.content.some((item) => item && item.type !== "text");
    if (hasPrompt || (!hasReference && body.content.length === 0)) body.content.push({ type: "text", text: req.prompt || "" });
  }
  const hasReference = body.content.some((item) => item && item.type !== "text");
  validateSeedanceContent(body.content);
  if (Array.isArray(body.content)) body.content = rewriteDraftTaskContent(body.content, ctx.originTasks);
  delete body.seconds;
  const secondsValue = req.seconds === undefined ? req.duration : req.seconds;
  if (secondsValue !== undefined) body.duration = validateSeedanceDuration(secondsValue);
  body.model = normalizeRunyuanModel(ctx.upstreamModel || body.model);
  return {
    url: baseUrl + "/v1/video/tasks",
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json", Authorization: "Bearer " + ctx.apiKey },
    body: body,
    action: hasReference ? "image_to_video" : "text_to_video",
    rewriteModel: body.model,
  };
}

// Used by the administrator's channel-test action. Runyuan does not publish a
// model-discovery endpoint; querying a sentinel task id validates the bearer
// credential without creating a generation task. A documented 404 is treated
// as healthy by the host when the endpoint responds with an error envelope.
export function buildChannelTestRequest(ctx) {
  const baseUrl = trimmed(ctx && ctx.baseUrl).replace(/\/+$/, "");
  const model = normalizeRunyuanModel(ctx && (ctx.upstreamModel || ctx.model));
  if (!baseUrl) throw new Error("baseUrl is required");
  if (!model) throw new Error("model is required");
  return {
    url: baseUrl + "/v1/video/tasks/channel-test-nonexistent",
    method: "GET",
    headers: { Accept: "application/json", Authorization: "Bearer " + trimmed(ctx && ctx.apiKey) },
    acceptedStatusCodes: [404],
    acceptErrorResponse: true,
  };
}

export function parseSubmitResponse(ctx, resp) {
  const body = resp && resp.body && typeof resp.body === "object" ? resp.body : {};
  const taskId = trimmed(body.task_id || body.id || (body.data && body.data.task_id) || (body.data && body.data.id));
  if (!taskId) throw new Error("task_id is empty");
  return { taskId: taskId, taskData: body };
}

export function extractUsage(ctx) {
  const req = ctx.requestBody || {};
  const metadata = req.metadata && typeof req.metadata === "object" && !Array.isArray(req.metadata) ? req.metadata : {};
  const requestContent = contentFromRequest(req);
  if (ctx.usagePurpose === "billing_ratios") {
    const ratio = videoInputRatio(ctx.upstreamModel || ctx.model, metadata.resolution || req.resolution || req.size, requestContent);
    return ratio === 1 ? null : { video_input_ratio: ratio };
  }
  let seconds = Number(req.seconds || req.duration || metadata.duration || 0);
  if (!Number.isFinite(seconds) || seconds <= 0) {
    const frames = Number(metadata.frames);
    seconds = Number.isFinite(frames) && frames > 0 ? Math.floor(frames / 24) : 15;
  }
  if (seconds <= 0) seconds = 5;
  seconds = Math.min(seconds, 3600);
  const rawResolution = metadata.resolution || req.resolution || req.size;
  const raw = trimmed(rawResolution).toLowerCase();
  const recognized = ["480p", "720p", "1080p"].includes(raw) || raw.replace("*", "x").split("x").length === 2;
  const resolution = recognized ? normalizeResolution(rawResolution) : "1080p";
  return {
    tokens: estimateTokens(seconds, resolution),
    resolution: resolution,
    video_input: hasVideo(requestContent) ? "video" : "none",
  };
}

export function buildQueryRequest(ctx) {
  const baseUrl = trimmed(ctx && ctx.baseUrl).replace(/\/+$/, "");
  if (!baseUrl) throw new Error("baseUrl is required");
  const taskId = trimmed(ctx && ctx.taskId);
  if (!taskId) throw new Error("taskId is required");
  return {
    url: baseUrl + "/v1/video/tasks/" + encodeURIComponent(taskId),
    method: "GET",
    headers: { Accept: "application/json", "Content-Type": "application/json", Authorization: "Bearer " + ctx.apiKey },
  };
}

export function parseTaskResult(ctx, body) {
  if (body.status === "pending" || body.status === "queued") return { status: "QUEUED", progress: "10%" };
  if (body.status === "processing" || body.status === "running") return { status: "IN_PROGRESS", progress: "50%" };
  if (body.status === "succeeded") {
    const result = { status: "SUCCESS", progress: "100%", url: body.content && body.content.video_url ? body.content.video_url : "" };
    const usage = body.usage || {};
    const completionTokens = Number(usage.completion_tokens || 0);
    const totalTokens = Number(usage.total_tokens || 0);
    if (Number.isFinite(completionTokens) && completionTokens > 0) result.completionTokens = completionTokens;
    if (Number.isFinite(totalTokens) && totalTokens > 0) result.totalTokens = totalTokens;
    return result;
  }
  if (body.status === "failed" || body.status === "expired" || body.status === "cancelled") {
    const reason = body.error && body.error.message ? body.error.message : body.status;
    return { status: "FAILURE", progress: "100%", reason: reason };
  }
  return { status: "IN_PROGRESS", progress: "30%" };
}

function artifactData(ctx) {
  const data = (ctx && ctx.data) || {};
  if (data.data && typeof data.data === "object" && data.data.task_id && Object.prototype.hasOwnProperty.call(data.data, "data")) return data.data.data || {};
  return data;
}

export function listArtifacts(task) {
  if (task.status !== "SUCCESS") return [];
  const content = artifactData(task).content || {};
  const artifacts = [];
  if (trimmed(content.video_url)) artifacts.push({ key: "video", type: "video" });
  if (trimmed(content.last_frame_url)) artifacts.push({ key: "last_frame", type: "image", mimeType: "image/png" });
  return artifacts;
}

export function buildContentRequest(ctx) {
  const content = artifactData(ctx).content || {};
  const urls = { video: content.video_url, last_frame: content.last_frame_url };
  const url = trimmed(urls[ctx.artifactKey]);
  if (!url) throw new Error("artifact_not_found");
  // Runyuan's CDN serves media with GET but does not implement HEAD.
  // The host still suppresses the response body for an incoming HEAD request,
  // so always use GET upstream while preserving OpenAI HEAD semantics.
  return { url: url, method: "GET", credentialless: true };
}

export function extractUsageOnComplete(task, taskResult, body) {
  if (!body || body.status !== "succeeded") return {};
  const facts = {};
  const usage = body.usage || {};
  let tokens = Number(usage.completion_tokens);
  if (!Number.isFinite(tokens) || tokens <= 0) tokens = Number(usage.total_tokens);
  if (Number.isFinite(tokens) && tokens > 0) facts.tokens = tokens;
  const content = body.content || {};
  const resolution = trimmed(content.resolution || body.resolution).toLowerCase();
  if (["480p", "720p", "1080p"].includes(resolution)) facts.resolution = resolution;
  return facts;
}

export const protocols = {
  openai_responses: {
    decodeRequest: function (ctx) {
      if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
      const req = ctx.body.value;
      if (!req || typeof req !== "object" || Array.isArray(req)) throw new Error("request body must be an object");
      const model = trimmed(req.model);
      if (!model) throw new Error("model is required");
      if (req.input !== undefined && typeof req.input !== "string" && !Array.isArray(req.input)) throw new Error("input must be a string or array");
      if (req.images !== undefined && !Array.isArray(req.images)) throw new Error("images must be an array");
      if (req.metadata !== undefined && (!req.metadata || typeof req.metadata !== "object" || Array.isArray(req.metadata)))
        throw new Error("metadata must be an object");
      const input = responsesInput(req);
      const prompt = input.prompt || trimmed(req.prompt);
      const images = [];
      for (const image of [req.image, req.input_reference].concat(req.images || [], input.images)) {
        if (trimmed(image) && !images.includes(trimmed(image))) images.push(trimmed(image));
      }
      if (!prompt && images.length === 0) throw new Error("input is required");
      const metadata = Object.assign({}, req.metadata || {});
      if (Object.prototype.hasOwnProperty.call(req, "resolution")) metadata.resolution = req.resolution;
      else if (req.size && !metadata.resolution) metadata.resolution = normalizeResolution(req.size);
      const requestBody = { model: model, prompt: prompt, metadata: metadata };
      if (images.length) requestBody.images = images;
      if (Object.prototype.hasOwnProperty.call(req, "seconds")) requestBody.seconds = validateSeedanceDuration(req.seconds);
      else if (Object.prototype.hasOwnProperty.call(req, "duration")) requestBody.seconds = validateSeedanceDuration(req.duration);
      if (Object.prototype.hasOwnProperty.call(req, "size")) requestBody.size = req.size;
      for (const key of ["generate_audio", "ratio", "watermark"]) {
        if (Object.prototype.hasOwnProperty.call(req, key)) requestBody[key] = req[key];
      }
      const intent = { kind: "submit", model: model, action: images.length ? "image_to_video" : "text_to_video", requestBody: requestBody };
      const originTaskIds = draftTaskIds(metadata.content);
      if (originTaskIds.length) intent.originTaskIds = originTaskIds;
      return intent;
    },
    renderEvents: function (ctx, task, previousState) {
      const status = String(task.status || "UNKNOWN").toUpperCase();
      const value = Number(String(task.progress || "").replace("%", ""));
      const progress = Number.isFinite(value) && value >= 0 && value <= 100 ? value : null;
      const state = { status: status, progress: progress };
      if (status === "SUCCESS") {
        const text = responsesVideoText(ctx);
        const events = previousState && previousState.status === status ? [] : [{ type: "output", data: text }];
        return { events: events, state: state, done: true };
      }
      if (status === "FAILURE")
        return { events: [{ type: "error", code: "task_failed", message: task.fail_reason || "task failed" }], state: state, done: true };
      if (previousState && previousState.status === status && previousState.progress === progress) return { events: [], state: state, done: false };
      const event = { type: "progress", message: status.toLowerCase() };
      if (progress !== null) event.progress = progress;
      return { events: [event], state: state, done: false };
    },
    renderFinal: function (ctx, _task) {
      return {
        output: [
          {
            type: "message",
            status: "completed",
            role: "assistant",
            content: [{ type: "output_text", text: responsesVideoText(ctx), annotations: [], logprobs: [] }],
          },
        ],
        metadata: { vendor: "runyuan" },
      };
    },
  },
};

const legacyRenderers = {
  openai_video: function (task) {
    const data = task.data || {};
    const statusMap = { NOT_START: "queued", SUBMITTED: "queued", QUEUED: "queued", IN_PROGRESS: "in_progress", SUCCESS: "completed", FAILURE: "failed" };
    const output = {
      id: task.task_id,
      object: "video",
      model: task.properties ? task.properties.origin_model_name || "" : "",
      status: statusMap[task.status] || "unknown",
      progress: Number(String(task.progress || "0").replace("%", "")),
      created_at: task.created_at,
      completed_at: task.updated_at,
    };
    if (data.status === "failed" || data.status === "cancelled") {
      output.error = {
        message: data.error ? data.error.message || "" : data.status === "cancelled" ? "cancelled" : "",
        code: data.error ? data.error.code || "" : data.status === "cancelled" ? "task_cancelled" : "",
      };
    }
    return output;
  },
};

protocols.openai_video = {
  decodeRequest: function (ctx) {
    if (!ctx.body || (ctx.body.kind !== "json" && ctx.body.kind !== "multipart")) throw new Error("JSON or multipart body required");
    if (ctx.body.kind === "json") {
      if (!ctx.body.value || Array.isArray(ctx.body.value)) throw new Error("JSON object required");
      const req = ctx.body.value;
      const isArkPath = String(ctx.path || "").includes("/v1/video/tasks");
      if (isArkPath && (!Array.isArray(req.content) || req.content.length === 0)) throw new Error("content must be a non-empty array");
      if (Array.isArray(req.content) && req.content.length > 5) throw new Error("content must contain at most 5 items");
      if (isArkPath) validateSeedanceContent(req.content);
      const seconds = req.seconds === undefined ? req.duration : req.seconds;
      if (seconds !== undefined) validateSeedanceDuration(seconds);
      const content = Array.isArray(req.content) ? req.content : [];
      const hasReference = content.some((item) => item && typeof item === "object" && item.type !== "text") || req.input_reference || req.image;
      return {
        kind: "submit",
        model: ctx.model,
        action: hasReference ? "image_to_video" : "text_to_video",
        requestBody: Object.assign({}, req, { model: ctx.model }),
      };
    }
    const first = function (name) {
      const values = (ctx.body.fields || {})[name] || [];
      if (values.length > 1) throw new Error(name + " must be provided once");
      return values[0];
    };
    const req = {};
    const fields = ctx.body.fields || {};
    for (const name of Object.keys(fields)) {
      req[name] = first(name);
    }
    if (req.metadata !== undefined) {
      let parsed;
      try {
        parsed = JSON.parse(req.metadata);
      } catch (e) {
        throw new Error("metadata must be a JSON object string");
      }
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error("metadata must be a JSON object string");
      req.metadata = parsed;
    }
    if ((ctx.body.files || []).length) throw new Error("Runyuan requires image and video references to be public URLs inside metadata.content");
    if (req.seconds !== undefined) req.seconds = Number(req.seconds);
    else if (req.duration !== undefined) req.seconds = Number(req.duration);
    const seconds = req.seconds === undefined ? req.duration : req.seconds;
    if (seconds !== undefined) req.seconds = validateSeedanceDuration(seconds);
    return {
      kind: "submit",
      model: ctx.model,
      action: req.input_reference || req.image ? "image_to_video" : "text_to_video",
      requestBody: Object.assign({}, req, { model: ctx.model }),
    };
  },
  render: function (ctx, task) {
    return legacyRenderers.openai_video(task);
  },
};
