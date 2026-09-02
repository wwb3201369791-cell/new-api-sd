import { describe, expect, it } from 'vitest'

import { findCollection, normalizeAssets, normalizeGroups } from '../lib/normalize'

describe('asset response normalization', () => {
  it('unwraps nested provider data arrays', () => {
    const groups = normalizeGroups({ body: { data: { items: [{ groupId: 'g1' }] } } })
    expect(groups).toEqual([{ groupId: 'g1' }])
  })

  it('accepts direct arrays and ignores scalar payloads', () => {
    expect(normalizeAssets([{ assetId: 'a1' }, 'invalid'])).toEqual([{ assetId: 'a1' }])
    expect(findCollection({ message: 'ok' })).toEqual([])
  })
})

