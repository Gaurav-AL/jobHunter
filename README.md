# JobHunter

JobHunter is a prototype job-opening assistant. The backend scrapes configured career sources, generates job/application drafts, and exposes review APIs. The frontend gives a mobile-friendly dashboard for login, personalization, source setup, resume upload, and apply/reject review.

## One-command local prototype

1. Configure the Firebase web app in `frontend/.env` and set the matching Firebase project id in `backend/.env`:

   ```sh
   cp frontend/.env.example frontend/.env
   cp backend/.env.example backend/.env
   ```

   `FIREBASE_PROJECT_ID` in the backend must match `VITE_FIREBASE_PROJECT_ID` in the frontend. Fill in your Firebase web app values before registering.

2. Start the complete stack:

   ```sh
   ./scripts/run-local.sh
   ```

3. Open the frontend:

   ```text
   http://localhost:3000
   ```

The compose stack starts MySQL, the Go API, and the Vue frontend. Nginx serves the frontend at `http://localhost:3000` and proxies `/api/*` to the backend.

## Live job discovery and applications

JobHunter reads published listings from company Greenhouse, Lever, and Ashby boards through their public job APIs. Add a direct company board URL such as `https://boards.greenhouse.io/company`, `https://jobs.lever.co/company`, or `https://jobs.ashbyhq.com/company`. Add the built-in Himalayas feed for country-filtered India remote jobs and Jobicy for APAC remote jobs. Public ATS APIs require a company board URL; they do not expose a global index of every employer. The profile location is used as a ranking preference, with India and common Indian city locations receiving a boost.

Listings are refreshed every six hours and can also be checked with **Run Hunt Now**. The dashboard shows when a listing was last seen on the board and hides jobs after a successful refresh no longer returns them. Each match has a tailored plain-text resume draft and application message. Review/download the resume, open the employer's official application form, submit it there, then use **I submitted the application** to record your action. JobHunter does not claim submission to arbitrary employer systems: ATS application endpoints require employer-issued credentials or each employer's configured form fields.

### Stored applicant details

Resume text is saved during upload and sent to the configured AICC/Gemini provider for structured profile extraction. Extracted fields fill only blank profile values; edit or correct them in **Personalisation → Stored applicant details**. Work authorization, visa sponsorship, notice period, and salary are never inferred and must be entered by you. Resume text and extracted details remain in your MySQL user record. The project highlights field is prefilled with this JobHunter project and included in generated drafts only when relevant to the role or when project experience is requested. Verify every generated answer before applying. The current app prepares applicant values for reuse; it does not inject them into or submit external employer forms.

### SMS notifications

Configure these values in the ignored local `backend/.env` (copy `backend/.env.example` first): `FIREBASE_PROJECT_ID` must match the frontend Firebase project; Twilio credentials need a Verify service and a Messaging Service SID or verified sender number. In Settings, enter your mobile in E.164 format, request and enter the SMS verification code, opt into job alerts, and save. SMS delivery cannot work until valid Twilio credentials and an eligible sender are configured. Provider trial accounts can restrict recipients. Never commit `.env` or paste Twilio secrets into chat.

### Security and data handling

API routes require a Firebase ID token; the backend verifies its signature and project claims and derives the user identity from that token. Job sources are restricted to official ATS hostnames to avoid arbitrary URL fetches. Resume uploads are limited to 12 MiB and stored in the backend uploads volume. Resume text and tailored drafts contain personal data; use private infrastructure, HTTPS, backups with access controls, and a retention policy before deploying publicly. Configure production secrets outside source control and replace the development MySQL credentials in `docker-compose.yaml` before internet exposure.

### Known boundaries

- Resume tailoring currently produces a reviewable plain-text draft. It may need formatting in a document editor before upload; always verify it against the original resume.
- External application forms differ by employer. The ATS job-listing APIs do not provide one shared form-fill endpoint; Greenhouse submission requires employer-side API credentials, and Lever application forms have posting-specific fields. Open the official application page and review each answer.
- JobHunter tracks that you clicked the manual confirmation button; it cannot independently verify a completed employer application.
- A public listing being present in an ATS response is evidence it is listed there at refresh time, not a guarantee the employer is still accepting submissions.
- SMS provider acceptance is not the same as confirmed handset delivery. Twilio's message status can remain queued or fail due to carrier/provider rules.


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
