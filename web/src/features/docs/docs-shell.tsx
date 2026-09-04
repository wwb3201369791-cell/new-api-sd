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
import {
  BookOpen,
  ChevronRight,
  Copy,
  ExternalLink,
  ShieldCheck,
} from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { ROLE } from '@/lib/roles'
import { cn } from '@/lib/utils'
import { useAuthStore } from '@/stores/auth-store'

import { DOCS_NAV_GROUPS, getDocsPageId, type DocsPage } from './content'
import { getArticleHeadings } from './docs-article-headings'
import { DocsPageArticle } from './docs-page'

type DocsShellProps = {
  page: DocsPage
  accessDenied?: boolean
}

function DocsNavLink({
  href,
  title,
  active,
}: {
  href: string
  title: string
  active: boolean
}) {
  const { t } = useTranslation()
  return (
    <a
      href={href}
      aria-current={active ? 'page' : undefined}
      className={cn(
        'flex items-center justify-between rounded-lg px-3 py-2 text-sm transition-colors',
        active
          ? 'bg-primary/10 text-primary font-medium'
          : 'text-muted-foreground hover:bg-muted hover:text-foreground'
      )}
    >
      <span>{t(title)}</span>
      {active && <ChevronRight className='size-3.5' aria-hidden='true' />}
    </a>
  )
}

function AccessDenied({ page }: { page: DocsPage }) {
  const { t } = useTranslation()
  return (
    <div className='bg-muted/40 rounded-xl border p-6'>
      <div className='flex items-start gap-3'>
        <ShieldCheck className='text-muted-foreground mt-0.5 size-5 shrink-0' />
        <div className='space-y-2'>
          <h2 className='text-lg font-semibold'>
            {t('Administrator access required')}
          </h2>
          <p className='text-muted-foreground text-sm'>
            {t('This page contains administrator operating guidance.')}
          </p>
          <a
            href='/docs/guide/user'
            className='text-primary text-sm hover:underline'
          >
            {t('Open the user guide')}
          </a>
        </div>
      </div>
      <p className='text-muted-foreground mt-4 text-xs'>
        {t('Requested page')}: {t(page.title)}
      </p>
    </div>
  )
}

export function DocsShell({ page, accessDenied = false }: DocsShellProps) {
  const { t } = useTranslation()
  const pathname = window.location.pathname
  const { auth } = useAuthStore()
  const canSeeAdmin = (auth.user?.role ?? ROLE.GUEST) >= ROLE.ADMIN
  const { copiedText, copyToClipboard } = useCopyToClipboard()
  const headings = useMemo(() => getArticleHeadings(page), [page])

  const visibleGroups = useMemo(
    () =>
      DOCS_NAV_GROUPS.map((group) => ({
        ...group,
        items: group.items.filter(
          (item) => item.id !== 'guide/admin' || canSeeAdmin
        ),
      })).filter((group) => group.items.length > 0),
    [canSeeAdmin]
  )

  const activePageId = getDocsPageId(
    pathname === '/docs' ? undefined : pathname.replace(/^\/docs\/?/, '')
  )

  return (
    <PublicLayout showMainContainer={false}>
      <div className='min-h-[calc(100svh-4rem)] pt-16'>
        <div className='border-b'>
          <div className='mx-auto flex max-w-7xl items-center gap-5 overflow-x-auto px-4 py-3 text-sm md:px-6'>
            <a
              href='/docs'
              className='text-foreground flex shrink-0 items-center gap-2 font-semibold'
            >
              <BookOpen className='text-primary size-4' aria-hidden='true' />
              {t('Documentation')}
            </a>
            <div className='bg-border h-4 w-px shrink-0' />
            {visibleGroups.map((group) => (
              <a
                key={group.title}
                href={group.items[0]?.href ?? '/docs'}
                className='text-muted-foreground hover:text-foreground shrink-0 transition-colors'
              >
                {t(group.title)}
              </a>
            ))}
            <a
              href='/docs/api'
              className='text-muted-foreground hover:text-foreground ml-auto flex shrink-0 items-center gap-1 transition-colors'
            >
              {t('API reference')}
              <ExternalLink className='size-3.5' aria-hidden='true' />
            </a>
          </div>
        </div>

        <div className='mx-auto grid max-w-7xl gap-8 px-4 py-8 md:grid-cols-[220px_minmax(0,1fr)_180px] md:px-6'>
          <aside className='hidden md:block'>
            <div className='sticky top-24 space-y-6'>
              {visibleGroups.map((group) => (
                <section key={group.title}>
                  <h2 className='text-muted-foreground mb-2 px-3 text-xs font-semibold tracking-wider uppercase'>
                    {t(group.title)}
                  </h2>
                  <nav className='space-y-1' aria-label={t(group.title)}>
                    {group.items.map((item) => (
                      <DocsNavLink
                        key={item.id}
                        href={item.href}
                        title={item.title}
                        active={item.id === activePageId}
                      />
                    ))}
                  </nav>
                </section>
              ))}
            </div>
          </aside>

          <main className='min-w-0'>
            <div className='mb-6 flex flex-wrap items-start justify-between gap-4'>
              <div>
                <p className='text-muted-foreground mb-2 text-sm'>
                  {t(page.summary)}
                </p>
                <h1 className='text-3xl font-semibold tracking-tight'>
                  {t(page.title)}
                </h1>
              </div>
              <Button
                variant='outline'
                size='sm'
                onClick={() => void copyToClipboard(page.markdown)}
                aria-label={t('Copy Markdown')}
              >
                <Copy className='size-3.5' aria-hidden='true' />
                {copiedText === page.markdown
                  ? t('Copied')
                  : t('Copy Markdown')}
              </Button>
            </div>
            {accessDenied ? (
              <AccessDenied page={page} />
            ) : (
              <DocsPageArticle page={page} />
            )}
          </main>

          <aside className='hidden lg:block'>
            {headings.length > 0 && !accessDenied && (
              <nav
                className='sticky top-24 space-y-2'
                aria-label={t('Table of contents')}
              >
                <h2 className='text-muted-foreground text-xs font-semibold tracking-wider uppercase'>
                  {t('On this page')}
                </h2>
                <div className='border-muted space-y-1 border-l pl-3'>
                  {headings.map((heading) => (
                    <a
                      key={heading.id}
                      href={`#${heading.id}`}
                      className='text-muted-foreground hover:text-foreground block text-xs leading-relaxed transition-colors'
                    >
                      {heading.title}
                    </a>
                  ))}
                </div>
              </nav>
            )}
          </aside>
        </div>
      </div>
    </PublicLayout>
  )
}
