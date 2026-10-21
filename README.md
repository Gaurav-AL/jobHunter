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

Listings are refreshed every six hours and can also be checked with **Run Hunt Now**. The dashboard shows when a listing was last seen on the board and hides jobs after a successful refresh no longer returns them. Use **Prepare application** on a matched role to create a reviewable tailored-resume copy and application note, then submit on the employer's official site. If AICC is unavailable, the app saves a factual fallback. JobHunter does not submit to arbitrary employer systems: Greenhouse application POST requires an employer board API key, while Lever and Ashby require their form flows and authorized API access.

### Resume relevance and application tracker

Matched jobs are ranked using target-role overlap, skills explicitly listed in the profile/resume, India location preference, and small source-quality tie-breakers. Each result shows recognized job skills asked for and the candidate skills that overlap. The score is a heuristic for sorting, not a promise of interview likelihood; add specific skills and target roles under **Personalisation** to improve it. Use **Mark applied** after submitting on the employer's site; the saved status is user-reported, not verified by the company. **Export job tracker** downloads a per-account `.xlsx` workbook with a ranked **Jobs** tab and a **Dashboard** tab containing an applied/not-applied pie chart and live count formulas. The workbook can be opened in Excel or uploaded/imported into Google Sheets. Direct writing into a Google account is not enabled because the app has no Google Sheets OAuth connection configured.

### Stored applicant details

Resume text is saved during upload and sent to the configured AICC provider for structured profile extraction. Uploading a new readable resume refreshes resume-derived fields; application answers such as notice period, experience, expected compensation, work authorization, and sponsorship remain separately stored. Edit or correct extracted fields in **Personalisation → Stored applicant details**. Work authorization and visa sponsorship are never inferred. The requested application answers are prefilled as Immediate joiner, 2.2 years, and ₹10 LPA; change them in your profile if needed. Resume text and extracted details remain in your MySQL user record. Applicant values are not submitted to external employer forms by this app.

`AICC_MODEL` defaults to `gpt-4o-mini` and can be changed in the backend environment. AICC calls occur when extracting a newly uploaded resume or preparing application material, not for every daily job listing; the backend logs token counts when the provider returns usage data. Check the AICC account dashboard for live model availability and billed usage. The daily GitHub email currently uses the curated profile in `scripts/daily_job_digest.py`, while dashboard matching uses the latest extracted profile stored in MySQL.

### Temporary dashboard access

The sign-in page can email a one-time Firebase email-link sign-in. That session uses browser-session persistence and expires two hours after authentication; the backend also rejects email-link Firebase tokens after that two-hour window. The daily job email can include a dashboard button when the GitHub Actions variable `JOBHUNTER_DASHBOARD_URL` is set to the deployed app's HTTPS URL. When the Firebase web API key variable is configured, the workflow also requests a sign-in link from Firebase during the daily run.

To enable email-link sign-in, enable **Email/Password** and **Email link (passwordless sign-in)** in Firebase Authentication, then add the dashboard host (and `localhost` for local development, if needed) to Firebase's Authorized domains. The email link must be opened by the frontend app and the user must confirm the receiving email when completing sign-in. The dashboard button opens the app; use its sign-in page to request the one-time link. Use HTTPS for deployment.

### SMS notifications

Configure these values in the ignored local `backend/.env` (copy `backend/.env.example` first): `FIREBASE_PROJECT_ID` must match the frontend Firebase project; Twilio credentials need a Verify service and a Messaging Service SID or verified sender number. In Settings, enter your mobile in E.164 format, request and enter the SMS verification code, opt into job alerts, and save. SMS delivery cannot work until valid Twilio credentials and an eligible sender are configured. Provider trial accounts can restrict recipients. Never commit `.env` or paste Twilio secrets into chat.

Master resume files uploaded through the app are stored in the authenticated user's private Firebase Storage folder (`resumes/{uid}/...`). The backend temporarily receives the file to extract text, then keeps the extracted profile and the Firebase object path in MySQL. Configure the Storage bucket and deploy the root `storage.rules` file before uploading resumes; see [Firebase setup](docs/FIREBASE_SETUP.md).

### Security and data handling

API routes require a Firebase ID token; the backend verifies its signature and project claims and derives the user identity from that token. Job sources are restricted to official ATS hostnames to avoid arbitrary URL fetches. Resume files are limited to 12 MiB and stored under per-user private Firebase Storage paths; the backend receives a temporary copy for text extraction and deletes its scratch file after a successful save. Resume text and extracted profile data remain in MySQL and contain personal information; use private infrastructure, HTTPS, backups with access controls, and a retention policy before deploying publicly. Configure production secrets outside source control and replace the development MySQL credentials in `docker-compose.yaml` before internet exposure.

Email-link sessions are limited to two hours using the signed Firebase `auth_time` claim at the API boundary; client-side sign-out is an additional convenience, not the sole expiry control. Configure Firebase's authorized domains carefully and use HTTPS for any public deployment.

### Known boundaries

- JobHunter prepares reviewable application materials on request but does not submit applications. It ranks and links to live employer postings. Do not invent answers when a form requests details missing from the stored profile or resume.
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
