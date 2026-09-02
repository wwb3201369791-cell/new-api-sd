import type { Asset, AssetGroup } from '../types'

function asRecord(value: unknown): Record<string, unknown> | null {
  return typeof value === 'object' && value !== null
    ? (value as Record<string, unknown>)
    : null
}

export function findCollection(value: unknown): unknown[] {
  if (Array.isArray(value)) return value
  const record = asRecord(value)
  if (!record) return []
  for (const key of ['body', 'data', 'items', 'list', 'results', 'records']) {
    const nested = record[key]
    if (Array.isArray(nested)) return nested
    if (asRecord(nested)) {
      const result = findCollection(nested)
      if (result.length > 0) return result
    }
  }
  return []
}

export function normalizeGroups(value: unknown): AssetGroup[] {
  return findCollection(value).filter((item): item is AssetGroup => !!asRecord(item))
}

export function normalizeAssets(value: unknown): Asset[] {
  return findCollection(value).filter((item): item is Asset => !!asRecord(item))
}
