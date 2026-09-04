export function isVideoModel(model: string): boolean {
  return model.toLowerCase().includes('seedance')
}

export function normalizeEndpoint(
  sourceUrl: string | undefined,
  model: string
): string {
  const video = isVideoModel(model)
  const fallbackPath = video ? '/v1/videos' : '/v1/chat/completions'
  const fallback =
    typeof window === 'undefined'
      ? fallbackPath
      : `${window.location.origin}${fallbackPath}`
  const trimmed = sourceUrl?.trim()
  if (!trimmed) return fallback

  const withoutTrailingSlash = trimmed.replace(/\/+$/, '')
  if (video) {
    if (withoutTrailingSlash.endsWith('/v1/videos')) return withoutTrailingSlash
    if (withoutTrailingSlash.endsWith('/v1/chat/completions')) {
      return `${withoutTrailingSlash.slice(0, -'/v1/chat/completions'.length)}/v1/videos`
    }
    if (withoutTrailingSlash.endsWith('/v1')) {
      return `${withoutTrailingSlash}/videos`
    }
    return `${withoutTrailingSlash}/v1/videos`
  }

  if (withoutTrailingSlash.endsWith('/v1/chat/completions')) {
    return withoutTrailingSlash
  }
  if (withoutTrailingSlash.endsWith('/v1')) {
    return `${withoutTrailingSlash}/chat/completions`
  }
  return `${withoutTrailingSlash}/v1/chat/completions`
}

export function buildCurlCommand(args: {
  endpoint: string
  apiKey: string
  model: string
}): string {
  if (isVideoModel(args.model) || args.endpoint.endsWith('/v1/videos')) {
    return [
      `curl ${args.endpoint} \\`,
      '  -H "Content-Type: application/json" \\',
      `  -H "Authorization: Bearer ${args.apiKey}" \\`,
      `  -d '{"model":"${args.model}","content":[{"type":"text","text":"A short cinematic scene at sunset."}],"duration":5,"resolution":"720p","ratio":"16:9"}'`,
    ].join('\n')
  }

  return [
    `curl ${args.endpoint} \\`,
    '  -H "Content-Type: application/json" \\',
    `  -H "Authorization: Bearer ${args.apiKey}" \\`,
    `  -d '{"model":"${args.model}","messages":[{"role":"user","content":"Say hello in one sentence."}]}'`,
  ].join('\n')
}
