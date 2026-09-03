import { describe, expect, it } from 'vitest'

import { getApiErrorMessage } from '../api-error-message'

describe('getApiErrorMessage', () => {
  it('prefers the server response message over the generic HTTP error', () => {
    const error = Object.assign(
      new Error('Request failed with status code 502'),
      {
        response: { data: { message: 'asset provider connection failed' } },
      }
    )

    expect(getApiErrorMessage(error)).toBe('asset provider connection failed')
  })

  it('falls back to the native error message when no response message exists', () => {
    expect(getApiErrorMessage(new Error('network unavailable'))).toBe(
      'network unavailable'
    )
  })
})
