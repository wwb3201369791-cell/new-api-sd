import { describe, expect, it } from 'vitest'

import {
  buildCurlCommand,
  isVideoModel,
  normalizeEndpoint,
} from '../request-example'

describe('dashboard request example', () => {
  it('uses the video endpoint and payload for Seedance models', () => {
    expect(isVideoModel('doubao-seedance-2.0')).toBe(true)
    expect(
      normalizeEndpoint(
        'https://gateway.example/v1/chat/completions',
        'doubao-seedance-2.0'
      )
    ).toBe('https://gateway.example/v1/videos')

    const command = buildCurlCommand({
      endpoint: 'https://gateway.example/v1/videos',
      apiKey: 'BearerToken',
      model: 'doubao-seedance-2.0',
    })
    expect(command).toContain('/v1/videos')
    expect(command).toContain('"content"')
    expect(command).toContain('"duration":5')
    expect(command).not.toContain('"messages"')
  })

  it('keeps the chat endpoint and payload for ordinary models', () => {
    expect(isVideoModel('gpt-4o-mini')).toBe(false)
    expect(normalizeEndpoint('https://gateway.example/v1', 'gpt-4o-mini')).toBe(
      'https://gateway.example/v1/chat/completions'
    )

    const command = buildCurlCommand({
      endpoint: 'https://gateway.example/v1/chat/completions',
      apiKey: 'BearerToken',
      model: 'gpt-4o-mini',
    })
    expect(command).toContain('"messages"')
    expect(command).not.toContain('"content":[{"type":"text"')
  })
})
