# JobHunter

JobHunter is a job-opening assistant. The backend checks configured career sources, matches postings to a stored applicant profile, and exposes job review APIs. The frontend gives a mobile-friendly dashboard for login, personalization, source setup, resume upload, and direct employer application links.

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

JobHunter reads published listings from company Greenhouse, Lever, and Ashby boards through their public job APIs. Add a direct company board URL such as `https://boards.greenhouse.io/company`, `https://jobs.lever.co/company`, or `https://jobs.ashbyhq.com/company`. Add the built-in Himalayas feed for country-filtered India remote jobs and Jobicy for APAC remote jobs. Public ATS APIs require a company board URL; they do not expose a global index of every employer. Source checks and match ranking prioritize Greenhouse, then Lever/Ashby, with Himalayas/Jobicy lower. Himalayas currently says applying is free and that its paid Plus tier covers optional tools; the lower rank follows the user's source preference, not an application fee.

Listings are refreshed every six hours and can also be checked with **Run Hunt Now**. The dashboard shows when a listing was last seen on the board and hides jobs after a successful refresh no longer returns them. New runs do not create resume or cover-letter drafts; matched cards link to the official listing so you can apply directly. JobHunter does not claim submission to arbitrary employer systems: Greenhouse application POST requires a board API key, Lever and Ashby require their application form flows and authorized API access. Those are employer-controlled credentials/permissions and are not available from a public job listing.

### Stored applicant details

Resume text is saved during upload and sent to the configured AICC/Gemini provider for structured profile extraction. Extracted fields fill only blank profile values; edit or correct them in **Personalisation → Stored applicant details**. Work authorization and visa sponsorship are never inferred. The requested application answers are prefilled as Immediate joiner, 2.2 years, and ₹10 LPA; change them in your profile if needed. Resume text and extracted details remain in your MySQL user record. Applicant values are not used to invent missing facts or submitted to external employer forms by this app.

### SMS notifications

Configure these values in the ignored local `backend/.env` (copy `backend/.env.example` first): `FIREBASE_PROJECT_ID` must match the frontend Firebase project; Twilio credentials need a Verify service and a Messaging Service SID or verified sender number. In Settings, enter your mobile in E.164 format, request and enter the SMS verification code, opt into job alerts, and save. SMS delivery cannot work until valid Twilio credentials and an eligible sender are configured. Provider trial accounts can restrict recipients. Never commit `.env` or paste Twilio secrets into chat.

### Security and data handling

API routes require a Firebase ID token; the backend verifies its signature and project claims and derives the user identity from that token. Job sources are restricted to official ATS hostnames to avoid arbitrary URL fetches. Resume uploads are limited to 12 MiB and stored in the backend uploads volume. Resume text and extracted profile data contain personal information; use private infrastructure, HTTPS, backups with access controls, and a retention policy before deploying publicly. Configure production secrets outside source control and replace the development MySQL credentials in `docker-compose.yaml` before internet exposure.

### Known boundaries

- New runs do not create application drafts or submit applications. They rank and link to live employer postings. Do not invent answers when a form requests details missing from the stored profile or resume.
- External application forms differ by employer. The ATS job-listing APIs do not provide one shared form-fill endpoint; Greenhouse submission requires an employer-side board API key, and Lever/Ashby require their posting-specific form plus authorized API access. The current app does not have those permissions.
- Employer confirmation emails go directly to the address entered on the application when that employer sends them. JobHunter does not read inboxes or claim an application succeeded based only on opening a listing.
- The digest does not track ATS submissions. Its application success ratio is N/A, not 0%. Employer acknowledgement emails go directly to the email address on the application when the employer sends them; JobHunter cannot read or verify those messages.
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
