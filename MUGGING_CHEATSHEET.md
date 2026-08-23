# JobHunter Vue-to-Go Integration Guide

Use this as the rebuild map and debugging checklist for the whole app: Vue 3 + Vite + TypeScript, Firebase Auth, Go + Gin, GORM + MySQL, Docker Compose, scheduler/scraper, Google GenAI via `AICC_KEY`, and notifications.

## 0. Architecture In One Breath

Firebase owns authentication. Vue logs the user in, reads the Firebase UID, and calls Go. Go maps that Firebase UID to a MySQL `users.id`, stores preferences/sources/jobs/drafts with GORM, and the scheduler uses scraper + LLM + notification services to create drafts.

```text
Browser
  -> Vue/Vite
  -> Firebase Auth
  -> Go Gin API
  -> MySQL via GORM
  -> Scheduler
  -> Scraper
  -> Google GenAI using AICC_KEY
  -> Draft rows + notifications
```

Memory hook: **VFG-MSLN**

- **V**ue UI
- **F**irebase Auth
- **G**in API
- **M**ySQL/GORM
- **S**cheduler/Scraper
- **L**LM
- **N**otification

## 1. Current Repo Map

```text
backend/
  cmd/main.go                       app entrypoint, env loading, DB connection, CORS, routes
  internal/api/handlers.go          HTTP contracts and request/response logic
  internal/repository/models.go     GORM models
  internal/scheduler/cron.go        background loop
  internal/scraper/scraper.go       job scraping adapter
  internal/services/llm.go          Google GenAI client
  internal/services/notification.go notification adapter
  .env.example                      backend env template

frontend/
  src/firebase.ts                   Firebase SDK setup from VITE_ env vars
  src/api.ts                        API base URL helper
  src/router/index.ts               Firebase-aware route guard
  src/views/Login.vue               Firebase login/register + backend registration
  src/views/Dashboard.vue           draft listing/status actions
  src/views/Settings.vue            preferences, resume upload, sources
  .env.example                      frontend env template
```

## 2. Environment Rules

Backend env lives in `backend/.env` when running `go run ./cmd` from `backend`.

```bash
PORT=8080
DB_DSN="jobhunter_user:jobhunter_password@tcp(127.0.0.1:3306)/jobhunter?charset=utf8mb4&parseTime=True&loc=Local"
AICC_KEY=your_google_genai_or_aicc_key
```

Frontend env lives in `frontend/.env`, not `frontend/src/.env`.

```bash
VITE_API_BASE_URL=http://localhost:8080
VITE_FIREBASE_API_KEY=...
VITE_FIREBASE_AUTH_DOMAIN=...
VITE_FIREBASE_PROJECT_ID=...
VITE_FIREBASE_STORAGE_BUCKET=...
VITE_FIREBASE_MESSAGING_SENDER_ID=...
VITE_FIREBASE_APP_ID=...
VITE_FIREBASE_MEASUREMENT_ID=...
```

Rules to memorize:

- Backend secrets never use `VITE_`.
- Frontend runtime config must use `VITE_`.
- Vite only exposes env names that start with `VITE_`.
- Firebase web config is public client config, but it still belongs in env so projects can switch cleanly.
- Never return or log API key values. Return booleans like `api_key: true`.

## 3. Local Run Order

```bash
docker compose up -d

cd backend
go test ./...
go run ./cmd

cd ../frontend
npm install
npm run build
npm run dev
```

Expected local URLs:

```text
Backend:  http://localhost:8080
Frontend: http://localhost:5173
```

Health checks:

```bash
curl http://localhost:8080/ping
curl http://localhost:8080/api/integrations/health
curl http://localhost:8080/api/sources
```

`/api/integrations/health` should return:

```json
{
  "database": { "status": "ok" },
  "llm": {
    "provider": "google_genai",
    "model": "gemini-2.5-flash",
    "api_key": true
  }
}
```

## 4. Frontend Integration Pattern

### Firebase Setup

`frontend/src/firebase.ts` reads Vite env, fails early when required Firebase config is missing, and exports:

```ts
export const auth = getAuth(app)
export const db = getFirestore(app)
export const authReady = new Promise<void>(...)
```

Use `authReady` before relying on `auth.currentUser` after a page refresh.

### API Base URL

`frontend/src/api.ts` normalizes `VITE_API_BASE_URL`.

```ts
apiUrl('/api/drafts')
```

This prevents mixed hardcoded URLs like `localhost:8080` in components.

### Auth Flow

Register:

```text
createUserWithEmailAndPassword
  -> Firebase returns currentUser.uid
  -> POST /api/users/register
  -> backend stores email + firebase_uid
  -> Vue navigates to dashboard
```

Login:

```text
signInWithEmailAndPassword
  -> Firebase restores currentUser
  -> route guard waits for authReady
  -> Vue can call backend with firebase_uid
```

### User Identity Contract

The browser should not hardcode `user_id=1`.

Use:

```text
firebase_uid = auth.currentUser.uid
```

Go resolves:

```text
firebase_uid -> users.firebase_uid -> users.id
```

This keeps MySQL ids private to the backend and makes multi-user behavior possible.

## 5. Backend Integration Pattern

### Startup

`backend/cmd/main.go` does:

```text
load .env
read DB_DSN or use docker-compose fallback
gorm.Open(mysql.Open(dsn))
sqlDB.Ping()
AutoMigrate(models)
StartScheduler(db)
configure Gin + CORS
register routes
run on PORT
```

The explicit `Ping()` catches bad credentials, wrong host, stopped MySQL, and network issues at startup.

### API Map

```text
GET  /ping                         basic backend health
GET  /api/integrations/health      DB ping + LLM key presence
POST /api/users/register           create/find user by Firebase UID
POST /api/users/resume             upload resume for firebase_uid or user_id
POST /api/users/settings           save settings for firebase_uid or user_id
GET  /api/sources                  list allowed job sources
POST /api/sources                  add allowed job source
GET  /api/drafts?firebase_uid=...  list current user's drafts
POST /api/drafts/:id/status        update draft status
```

### Handler Shape

Every handler should follow:

```text
Bind/parse input
Validate identity
Call DB/service
Handle error
Return JSON
```

In code:

```go
if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
}
```

For user-specific endpoints:

```go
userID, err := resolveUserID(db, c.Query("user_id"), c.Query("firebase_uid"))
```

## 6. Database Integration

Docker Compose provides local MySQL:

```yaml
services:
  db:
    image: mysql:8.0
    environment:
      MYSQL_ROOT_PASSWORD: rootpassword
      MYSQL_DATABASE: jobhunter
      MYSQL_USER: jobhunter_user
      MYSQL_PASSWORD: jobhunter_password
    ports:
      - "3306:3306"
```

GORM DSN shape:

```text
user:password@tcp(host:port)/database?charset=utf8mb4&parseTime=True&loc=Local
```

Manual DB checks:

```bash
docker compose exec db mysql -ujobhunter_user -pjobhunter_password jobhunter
SHOW TABLES;
SELECT id,email,firebase_uid FROM users;
SELECT * FROM allowed_sources;
SELECT * FROM job_drafts;
```

Common DB failures:

- Docker is not running.
- Port `3306` is already used.
- `DB_DSN` points to `localhost` from inside a container; use service name `db` in container-to-container networking.
- Password in `DB_DSN` does not match `docker-compose.yaml`.
- MySQL is still booting; retry after logs show ready for connections.

## 7. LLM Integration

Current service:

```text
backend/internal/services/llm.go
env:   AICC_KEY
SDK:   google.golang.org/genai
model: gemini-2.5-flash
```

Generation flow:

```text
resume text + job description
  -> prompt
  -> genai.NewClient(ctx, APIKey)
  -> client.Models.GenerateContent(...)
  -> extract first candidate text
```

Debug checklist:

- `backend/.env` has `AICC_KEY`.
- Backend was restarted after changing `.env`.
- `/api/integrations/health` returns `"api_key": true`.
- Logs do not show `AICC_KEY environment variable is not set`.
- SDK model name matches the provider/account availability.
- Errors from the SDK are logged, but key values are never logged.

## 8. Scheduler/Scraper Flow

Every minute:

```text
load allowed_sources
for each source:
  ScrapeJob(source.URL)
  save job
  load users
  GenerateDraft(resumeText, job.Description)
  save JobDraft
  SendNotification(...)
```

Current scraper and notification services are placeholders. Production upgrades:

- Replace scraper mock with HTTP fetch + HTML parsing.
- Add duplicate protection for `jobs.url` and `(user_id, job_id)` drafts.
- Read actual resume text instead of dummy resume text.
- Move uploads from local `./uploads` to persistent storage.
- Replace notification logging with email/WhatsApp provider SDK.

## 9. End-To-End Test Checklist

Use this exact order when testing the full app.

1. Start Docker.
2. Run `docker compose up -d`.
3. Confirm MySQL: `docker compose ps`.
4. Start backend: `cd backend && go run ./cmd`.
5. Check `GET /ping`.
6. Check `GET /api/integrations/health`.
7. Start frontend: `cd frontend && npm run dev`.
8. Register a new Firebase email/password user.
9. Confirm backend `users` row has the Firebase UID.
10. Add a source in Settings.
11. Upload a resume.
12. Save preferences.
13. Wait for scheduler or trigger/create drafts manually during development.
14. Open Dashboard and confirm drafts load for the signed-in Firebase user.
15. Apply/reject a draft and confirm DB status changes.

Build/test commands:

```bash
cd backend && go test ./...
cd frontend && npm run build
```

## 10. Reading Any New SDK For Integration

Use this method for Firebase, GenAI, Stripe, Slack, storage providers, notification APIs, or any future SDK.

### Step 1: Find The Minimal Happy Path

Look for four things in the docs:

```text
install package
create client
authenticate client
call one method
```

Write a tiny isolated function first. Do not spread SDK calls across the app until the tiny function works.

### Step 2: Classify The Key

Ask:

```text
Is this secret server-side only?
Is this public browser config?
Does the SDK need a service account?
Does it use OAuth/user tokens?
```

Env placement:

```text
Secret backend key       -> backend/.env, no VITE_
Public frontend config   -> frontend/.env, VITE_
Service account JSON     -> backend secret file/env, never frontend
User token               -> request/session/Authorization header
```

### Step 3: Define The Boundary

Frontend SDKs are good for UI-owned work like Firebase Auth.

Backend SDKs are good for secrets, database writes, LLM calls, scraping, payments, and provider webhooks.

If the SDK requires a secret, it belongs in Go.

### Step 4: Wrap The SDK

Create one local adapter:

```text
internal/services/provider.go
```

Expose app-shaped functions:

```go
GenerateDraft(resumeText, jobDescription string) (string, error)
SendNotification(userID uint, email, phone, title string)
```

Do not let SDK-specific request objects leak into handlers unless the handler is only a webhook boundary.

### Step 5: Add A Health Signal

For every integration, add a non-secret health check:

```json
{
  "provider": "name",
  "configured": true,
  "status": "ok"
}
```

Never return actual tokens, DSNs, passwords, or private keys.

### Step 6: Map Errors To User Actions

Translate provider errors into useful categories:

```text
missing env
bad credentials
permission denied
quota/rate limit
network timeout
invalid request payload
provider unavailable
```

Logs can hold technical details. API responses should stay safe and understandable.

### Step 7: Test At Three Levels

```text
unit: adapter handles missing env/input
integration: health endpoint reaches provider or validates config
e2e: user action exercises frontend -> backend -> provider -> database
```

## 11. Most Common Bugs In This App

- `frontend/src/.env` is used by mistake. Use `frontend/.env`.
- Firebase config is hard-coded in `firebase.ts`. Use `VITE_FIREBASE_*`.
- Frontend hardcodes `user_id=1`. Send `firebase_uid`.
- Backend has `AICC_KEY` locally but the deployed service does not.
- MySQL runs locally, but deployed backend points at `127.0.0.1`.
- CORS allows local dev but not the deployed frontend domain.
- Scheduler creates duplicate drafts when run repeatedly.
- Uploads disappear after deploy because local disk is ephemeral.
- Firebase Auth works, but backend registration failed and no MySQL user row exists.

## 12. Deployment Checklist

Backend platform env:

```bash
PORT=8080
DB_DSN=managed_mysql_connection_string
AICC_KEY=provider_key
```

Frontend platform env:

```bash
VITE_API_BASE_URL=https://your-backend-domain.com
VITE_FIREBASE_API_KEY=...
VITE_FIREBASE_AUTH_DOMAIN=...
VITE_FIREBASE_PROJECT_ID=...
VITE_FIREBASE_STORAGE_BUCKET=...
VITE_FIREBASE_MESSAGING_SENDER_ID=...
VITE_FIREBASE_APP_ID=...
```

Before shipping:

- Run `go test ./...`.
- Run `npm run build`.
- Hit `/ping`.
- Hit `/api/integrations/health`.
- Register/login through Firebase.
- Confirm one user row appears in MySQL.
- Confirm no secrets are committed.
- Replace CORS `*` with exact frontend origin for production.

## 13. Oral Revision Script

"Vue uses Firebase Auth and waits for Firebase to restore the user. After register, Vue sends the Firebase UID to Go. Go stores that UID in MySQL and resolves future requests from `firebase_uid` to the internal `users.id`. Backend loads `.env`, pings MySQL, migrates GORM models, starts the scheduler, and exposes health checks. The scheduler reads sources, scrapes jobs, calls Google GenAI with `AICC_KEY`, stores drafts, and sends notifications. For any new SDK, I first identify install/client/auth/one-call, decide whether the key belongs frontend or backend, wrap it in one adapter, add a safe health signal, and test from unit to integration to e2e."
