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

import { isExternalDocumentationLink, normalizeDocsLink } from './docs-link'

describe('documentation link classification', () => {
  test('uses the local docs route for blank values', () => {
    expect(normalizeDocsLink('   ')).toBe('/docs')
    expect(isExternalDocumentationLink(undefined)).toBe(false)
  })

  test('keeps relative links inside the application', () => {
    expect(isExternalDocumentationLink('/docs')).toBe(false)
    expect(isExternalDocumentationLink('/docs/api')).toBe(false)
    expect(isExternalDocumentationLink('#api')).toBe(false)
  })

  test('marks configured absolute links as external', () => {
    expect(isExternalDocumentationLink('https://docs.example.com')).toBe(true)
    expect(isExternalDocumentationLink('//docs.example.com/guide')).toBe(true)
  })
})
