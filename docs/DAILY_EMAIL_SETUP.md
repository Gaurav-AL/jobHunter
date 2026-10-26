# Daily resume-matched email digest

The GitHub Actions workflow runs daily at 18:00 India Standard Time (12:30 UTC) and can also be started manually from the Actions tab. It searches Google Jobs through SerpApi for India-wide roles when configured, adds India-eligible remote roles from no-key feeds, and emails a dashboard plus matching jobs to `gauravchaudhary.hck@gmail.com`. The dashboard's job matching uses the profile extracted from the uploaded resume. The standalone GitHub digest currently uses the curated `PROFILE` near the top of `scripts/daily_job_digest.py`; it does not read your private MySQL/Firebase profile, so update that list if your target skills or roles change. The proposed cross-system profile sync is paused pending your approval because it would send extracted personal profile fields to the GitHub Actions runner.

## Required repository secrets

In **Gaurav-AL/jobHunter → Settings → Secrets and variables → Actions**, add:

- `SMTP_USER` — the Gmail account that sends the digest (can be the destination address)
- `SMTP_PASSWORD` — a Gmail App Password; do not use the normal account password
- `SERPAPI_API_KEY` — optional, for India-wide Google Jobs results. SerpApi's free plan allows 250 searches/month; the daily workflow makes six searches, about 180/month before manual runs.
- `JOBVETTA_API_KEY` — optional, for an additional India-only job feed (50 requests/day on its free tier).

In the **Variables** tab, set `JOBHUNTER_DASHBOARD_URL` to the deployed dashboard's HTTPS URL to add an **Open JobHunter dashboard** button to each digest, and set `FIREBASE_WEB_API_KEY` to the Firebase web app API key (it is public client config, not an admin credential). Do not set the dashboard variable to localhost for a remotely received email; that address points to the email reader's own device. The workflow requests a one-time Firebase email-link sign-in during the daily run; Firebase sends that link in its own email, alongside the digest. Access expires two hours after sign-in.

The Firebase sign-in message is delivered separately by Firebase Auth near the daily SMTP digest. Firebase controls the one-time link's validity period; the two-hour window starts after successful sign-in. Both dashboard variables must point to a real deployed HTTPS origin for mobile access. `localhost` only reaches the device opening the email.

No-key remote sources include Jobicy and Himalayas; these do not provide a complete local/on-site India index. Jobicy and Himalayas results retain their source links and attribution. SerpApi's free plan has a monthly quota; manual runs also consume it. Jobvetta requires a free account key and has a 50-request daily limit. The email sender must permit SMTP sending. If using Gmail, app passwords require 2-Step Verification; create a dedicated app password and revoke it if no longer needed. Never commit secrets or paste them into issue text.

## What to expect

- SerpApi searches six role queries with `location=India`; the search quota is limited to six calls per scheduled day. Optional Jobvetta adds up to 10 India jobs per request; Jobicy and Himalayas add remote roles.
- The email dashboard reports records returned by sources, resume-matched roles, submitted applications, and manual follow-up. Counts reflect only this run and duplicate listings are deduplicated before ranking.
- Only listings with a valid posted date within the last seven days and at least one resume-skill match are eligible.
- The email includes up to 100 roles. No company receives a preferred-company bonus and no company is excluded; roles are scored by resume-skill fit, role level, and freshness, then spread across distinct employers where the search results allow it.
- The number can be lower than 20 when the providers have fewer recent matches; the workflow reports this rather than inventing opportunities. If all sources return no listings, the workflow fails clearly.
- Search APIs provide job listings, not a universal application submission service. The workflow does not submit applications: there is no supported cross-company application endpoint, and employer forms require job-specific responses, consent choices, and often separate ATS credentials. The dashboard therefore reports 0 applied and sends every resume-matched job for manual follow-up under the 70% rule. It does not label unattempted applications as failed.

## Run once manually

Open **Actions → Daily JobHunter email → Run workflow** after adding the SMTP secrets and any optional job API keys. Inspect the workflow logs for source errors, fetched count, and email send result. Do not echo secret values in logs.

## Enable temporary email-link sign-in

In Firebase Authentication, enable both **Email/Password** and **Email link (passwordless sign-in)**. Add the app's production hostname to **Authorized domains**; add `localhost` separately for local testing. The email link is one-time and Firebase verifies the recipient's email. Enter the recipient email again when completing sign-in. The backend checks the signed Firebase authentication time and rejects email-link sessions older than two hours. Deploy the dashboard over HTTPS before setting `JOBHUNTER_DASHBOARD_URL`.
