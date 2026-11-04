# Coding Agent Prompt — Beeline Relay Website

## Your role

Act as a senior product designer and frontend engineer building Beeline Relay's public website. Read `AGENTS.md`, `docs/PRD.md`, `docs/TRD.md`, and `docs/MVP_SCOPE.md` first. Inspect the current repository before coding.

## Goal and positioning

Build a polished website explaining Beeline Relay as a **developer-facing payment reliability and reconciliation layer** for applications using supported cross-chain payment routes. It helps developers handle payment sessions, idempotency, lifecycle state, reconciliation and evidence provenance around the provider flow.

Do not position Beeline Relay as another consumer cross-chain payment app, a new bridge, or a replacement for NEAR Intents. Do not claim that generic status tracking, cross-chain routing, or payment links are novel.

Suggested core message: **Know what happened to every payment. Recover without guessing.**

Supporting message: Beeline Relay helps developers create parameter-bound payment sessions, track lifecycle events, reconcile uncertain outcomes, and export evidence—starting with one verified NEAR Intents route.

Only say “verified route” after the implementation has actually completed and documented an end-to-end test. Until then, use “designed to support one verified route” or “route verification in progress.”

## Stack and location

- Next.js App Router, TypeScript and Tailwind CSS.
- Website lives in `beeline-web/apps/website/` unless the existing repository uses a different convention.
- Developer console remains a separate app (currently expected at `beeline-web/apps/app/`).
- The Go API is separate; do not reimplement backend logic in the website.

## Suggested page sections

1. **Navigation:** logo/wordmark, Product, How it works, Reliability testing, Docs/Repository only if real links exist, and “Open console” CTA.
2. **Hero:** state the operational pain clearly: duplicate requests, stale quotes, delayed settlement, and uncertain outcomes are hard to handle consistently.
3. **How it works:** bind a quote to a session, record lifecycle events, reconcile against documented evidence, export the record.
4. **Payment lifecycle visual:** created → awaiting authorization → submitted → processing → completed/failed/recovery required. Label this a normalized illustrative lifecycle and note that the actual provider flow varies by route.
5. **Reliability test cases:** show the categories the test harness covers, such as idempotency conflict, stale observation and uncertain provider timeout. Do not invent pass rates or imply all scenarios have passed until tests confirm them.
6. **Evidence provenance:** explain the difference between provider-reported status and independently verified chain data.
7. **Security boundaries:** user wallet retains keys; provider secrets stay server-side; no blind retry on ambiguous value-moving operations. Claim only what the implementation satisfies.
8. **Limitations/FAQ:** one route first; support depends on provider documentation and availability; no settlement-time guarantee; not a new bridge.
9. **Footer:** functional links to console, docs and repository only when configured.

## Design direction

- High-quality developer-tooling / fintech infrastructure aesthetic.
- Strong typography, restrained color, crisp lifecycle diagrams, and an evidence/timeline visual.
- Avoid generic crypto stock imagery, fake real-time metrics, decorative transaction hashes, invented partner logos, fabricated testimonials and unsupported trust badges.
- Responsive, accessible semantic HTML, keyboard navigation, visible focus states, sufficient contrast and reduced-motion support.
- Avoid dead links, placeholder buttons and fake functionality.

## Functional requirements

- All navigation and CTA targets must be real/configurable.
- App URL configurable, for example `NEXT_PUBLIC_APP_URL`, with a documented local default.
- Use accurate metadata/title/description.
- Do not call the API for decorative fake data.
- If displaying route status or verification date, source it from a verified API/configuration value; do not hardcode a success claim.
- Product claims must match PRD and the implementation's current state.

## Acceptance criteria

- Responsive website builds and passes lint/type checks.
- Main CTA opens the developer console.
- Product narrative is about reliability, reconciliation and evidence, not generic cross-chain transfer UX.
- No fabricated metrics, pass rates, partnerships, audits, route support or guaranteed transaction timing.
- Lifecycle illustration is labelled illustrative and does not imply every provider route has identical states.
- Every interactive control has a real action or is plainly non-interactive.

## Working method

Inspect the existing repository, state a brief plan and implement incrementally. Test production build and lint/type checks. Report changed files, commands run, missing URLs/assets, claims that remain conditional, and any incomplete functionality.
