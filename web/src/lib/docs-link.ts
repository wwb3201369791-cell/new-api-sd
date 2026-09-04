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

export function normalizeDocsLink(value: unknown): string {
  if (typeof value !== 'string') return '/docs'
  const link = value.trim()
  return link || '/docs'
}

export function isExternalDocumentationLink(value: unknown): boolean {
  const link = normalizeDocsLink(value)
  if (
    (link.startsWith('/') && !link.startsWith('//')) ||
    link.startsWith('#') ||
    link.startsWith('?')
  ) {
    return false
  }

  try {
    const url = new URL(link, 'http://new-api.local')
    return url.origin !== 'http://new-api.local'
  } catch {
    return true
  }
}
