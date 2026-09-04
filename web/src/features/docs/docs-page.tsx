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
import { useEffect, useRef } from 'react'

import { Markdown } from '@/components/ui/markdown'

import { slugifyDocsHeading, type DocsPage } from './content'

type DocsPageArticleProps = {
  page: DocsPage
  markdown?: string
}

export function DocsPageArticle({
  page,
  markdown = page.markdown,
}: DocsPageArticleProps) {
  const articleRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const headings = articleRef.current?.querySelectorAll('h2, h3')
    if (!headings) return

    const usedIds = new Map<string, number>()
    headings.forEach((heading) => {
      const base = slugifyDocsHeading(heading.textContent ?? '')
      const count = usedIds.get(base) ?? 0
      usedIds.set(base, count + 1)
      heading.id = count === 0 ? base : `${base}-${count + 1}`
    })
  }, [markdown])

  return (
    <div ref={articleRef}>
      <Markdown>{markdown}</Markdown>
    </div>
  )
}
