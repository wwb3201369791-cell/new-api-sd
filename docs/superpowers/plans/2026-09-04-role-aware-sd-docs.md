# Implementation Plan: Role-aware SD documentation center

> **For implementation:** execute this plan inline in the current session, validating each task before moving to the next.

**Goal:** Replace the single external documentation link with an in-product documentation center that follows the New API information architecture, adapts content for Seedance/SD proxy usage, and exposes administrator-only operations without leaking operational data.

**Architecture:** Add a typed documentation content registry and audience gate in `web/src/features/docs`. Render all documentation pages through a shared responsive shell with grouped navigation, article Markdown, table of contents, copy-Markdown action, and role-aware route guards. Keep custom configured documentation URLs external while making the default link point to the local `/docs` center.

**Tech stack:** React, TanStack Router, existing i18n, existing `Markdown` component, Vitest, Go settings tests.

## Task 1: Add typed documentation content and audience rules

**Files:**
- Create `web/src/features/docs/content.ts`
- Create `web/src/features/docs/access.ts`
- Create `web/src/features/docs/content.test.ts`

1. Define page IDs, navigation groups, audience (`public`, `user`, `admin`) and typed page metadata.
2. Add concise adapted pages for overview, installation, user guide, administrator guide, Seedance usage, API reference, support, and policy. Include channel setup, video task polling, asset groups, public URL requirements, error mapping, and request tracing without copying upstream text verbatim.
3. Implement `canViewDocsPage(pageId, role)` using existing role constants; anonymous and normal users may read public/user pages, administrators may read admin pages, and unauthorized admin access must be denied.
4. Add tests covering page registry completeness, navigation targets, and role access for guest/user/admin/root.

## Task 2: Build the shared documentation shell

**Files:**
- Create `web/src/features/docs/docs-shell.tsx`
- Create `web/src/features/docs/docs-page.tsx`
- Create `web/src/features/docs/docs-shell.test.tsx`

1. Build the New API-style shell: top-level section navigation, grouped left sidebar, central Markdown article, right-side heading index, responsive collapse, and active-page highlighting.
2. Use existing UI primitives and `Markdown`; add copy-Markdown feedback through the existing clipboard hook.
3. Render an access-denied state with a link to the user guide when a user opens an administrator page.
4. Keep all shell labels, status text, and buttons in i18n; keep documentation body content in the typed registry so it is versioned with the application.

## Task 3: Register documentation routes

**Files:**
- Create `web/src/routes/docs/index.tsx`
- Create `web/src/routes/docs/$page.tsx`

1. Add `/docs` home route and `/docs/$page` article route using the shared shell.
2. Resolve unknown page IDs to the docs home page rather than throwing a blank route error.
3. Pass the authenticated user role into the audience guard and preserve the existing application layout and login behavior.
4. Verify TanStack route generation does not require hand-editing generated files.

## Task 4: Make the top navigation default local and preserve custom external links

**Files:**
- Modify `setting/operation_setting/general_setting.go`
- Modify `web/src/hooks/use-top-nav-links.ts`
- Create or update the nearest existing hook test for docs-link classification

1. Change the default `DocsLink` setting to `/docs`.
2. Add a small pure helper that treats relative paths as internal and absolute non-local URLs as external.
3. Keep a configured custom URL unchanged and mark it external; use the local docs route only when the setting is blank or the default is selected.
4. Add unit coverage for relative, absolute, protocol-relative, blank, and malformed values.

## Task 5: Align repository documentation and public entry points

**Files:**
- Modify `docs/mobilecloud-seedance.md`
- Modify `docs/mobilecloud-seedance-usage.md`
- Modify `docs/runyuan-seedance.md`
- Modify `docs/mobilecloud-asset-curl.md`

1. Add a short “in-product docs” link and cross-links between administrator setup, user API usage, and asset API pages.
2. Document the public contract: one New API token for clients, provider credentials retained by administrators, default per-user asset group, optional group creation, and provider-specific mapping hidden behind the gateway.
3. Keep real credentials, host passwords, and deployment secrets out of repository files.

## Task 6: Add translations and run verification

**Files:**
- Modify `web/src/i18n/locales/zh.json`
- Modify `web/src/i18n/locales/en.json`
- Modify `web/src/i18n/static-keys.ts` only if required by the existing i18n loader

1. Add keys for docs navigation, copy action, access-denied state, and route fallback in both locales.
2. Run frontend formatting, typecheck, lint, and tests from `web`.
3. Run focused Go regression tests for controller, router, relay channel, and mobilecloud asset packages.
4. Run `git diff --check` and inspect the final diff for secrets and generated artifacts.

## Task 7: Commit, push, and deploy the verified build

**Files:**
- No additional source files; use repository deployment scripts and documented server paths.

1. Commit the implementation and verification changes with a descriptive message.
2. Fetch the GitHub remote, reconcile any non-fast-forward divergence without force-pushing, then push the complete current branch.
3. Build and deploy the same commit to the configured server using `/data` for persistent platform data; preserve a rollback copy.
4. Smoke-test `/docs`, `/docs/guide/seedance`, `/docs/api`, and an administrator page with both user and admin sessions.
5. Report the commit, deployment status, test commands, and the exact local routes available for manual testing.
