# JobHunter

JobHunter is a prototype job-opening assistant. The backend scrapes configured career sources, generates job/application drafts, and exposes review APIs. The frontend gives a mobile-friendly dashboard for login, personalization, source setup, resume upload, and apply/reject review.

## One-command local prototype

1. Put Firebase web app values in `frontend/.env`. You can start from the template:

   ```sh
   cp frontend/.env.example frontend/.env
   ```

2. Start the complete stack:

   ```sh
   ./scripts/run-local.sh
   ```

3. Open the frontend:

   ```text
   http://localhost:3000
   ```

The compose stack starts MySQL, the Go API, and the Vue frontend. Nginx serves the frontend and proxies `/api/*` to the backend, so the prototype uses one browser URL.

Useful health checks:

```sh
curl http://localhost:3000/ping
curl http://localhost:3000/api/integrations/health
```

## Local development without Docker

Run MySQL with Docker:

```sh
docker compose up db
```

Run the backend:

```sh
cd backend
go run ./cmd
```

Run the frontend:

```sh
cd frontend
npm install
npm run dev
```

The Vite dev server proxies `/api` and `/ping` to `http://localhost:8080`.
