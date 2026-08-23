# JobHunter Roadmap

## Phase 1: One-command prototype

- Package MySQL, backend, and frontend in Docker Compose.
- Serve the frontend at one URL and proxy API traffic through Nginx.
- Keep resume uploads in a Docker volume.
- Add API/database health checks so startup order is reliable.

## Phase 2: Shareable URL

- Deploy the same images to a small VPS, Render/Fly.io/Railway, or Cloud Run plus Cloud SQL.
- Put HTTPS and a stable domain in front of the frontend proxy.
- Move secrets to the platform secret manager.
- Configure Firebase authorized domains for the deployed URL.

## Phase 3: Real notification loop

- Replace mocked notification logging with WhatsApp, email, or push notification providers.
- Store notification delivery status and retry failures.
- Add user-level notification frequency enforcement.

## Phase 4: Job discovery quality

- Convert the current scraper mock into source-specific adapters.
- Add deduplication, job freshness checks, and source health monitoring.
- Add admin controls for global sources and user-specific sources.

## Phase 5: Review and application workflow

- Add draft preview/edit before apply.
- Track applied/rejected history with job metadata.
- Add mobile-first quick actions from notifications.

## Phase 6: Production hardening

- Add structured logging, request IDs, and metrics.
- Add backend tests around user resolution, settings, source management, and draft status changes.
- Add frontend smoke tests for login, settings, and dashboard flows.
- Add CI to build images and run tests on every change.
