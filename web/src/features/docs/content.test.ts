/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { describe, expect, test } from 'vitest'

import { canViewDocsPage } from './access'
import {
  DOCS_NAV_GROUPS,
  DOCS_PAGES,
  getDocsHeadings,
  getDocsPage,
  slugifyDocsHeading,
} from './content'

describe('documentation registry', () => {
  test('contains every navigation target', () => {
    const pageIds = new Set(Object.keys(DOCS_PAGES))
    const navigationIds = DOCS_NAV_GROUPS.flatMap((group) =>
      group.items.map((item) => item.id)
    )

    expect(navigationIds.every((id) => pageIds.has(id))).toBe(true)
    expect(navigationIds).toContain('guide/seedance')
    expect(getDocsPage('unknown-page').id).toBe('overview')
  })

  test('gates administrator guidance by role', () => {
    expect(canViewDocsPage('overview')).toBe(true)
    expect(canViewDocsPage('guide/user', 0)).toBe(true)
    expect(canViewDocsPage('guide/admin', 0)).toBe(false)
    expect(canViewDocsPage('guide/admin', 1)).toBe(false)
    expect(canViewDocsPage('guide/admin', 10)).toBe(true)
    expect(canViewDocsPage('guide/admin', 100)).toBe(true)
  })

  test('extracts stable heading anchors', () => {
    const headings = getDocsHeadings(DOCS_PAGES.api.markdown)
    expect(headings).toContain('Authentication')
    expect(slugifyDocsHeading('Use `asset://ASSET_ID`')).toBe(
      'use-asset-asset-id'
    )
  })
})
