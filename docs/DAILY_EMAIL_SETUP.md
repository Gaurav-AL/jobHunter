# Daily resume-matched email digest

The GitHub Actions workflow runs daily at 08:47 India Standard Time and can also be started manually from the Actions tab. It searches Adzuna's India job listing API for software engineering roles, ranks recent matches against the skills and experience represented in Gaurav Chaudhary's supplied resume, filters out clear senior/lead/management jobs, and emails up to 20 current matches to `gauravchaudhary.hck@gmail.com`.

## Required repository secrets

In **Gaurav-AL/jobHunter → Settings → Secrets and variables → Actions**, add:

- `ADZUNA_APP_ID`
- `ADZUNA_APP_KEY`
- `SMTP_USER` — the Gmail account that sends the digest (can be the destination address)
- `SMTP_PASSWORD` — a Gmail App Password; do not use the normal account password

The Adzuna API requires a registered app id and key. The email sender must permit SMTP sending. If using Gmail, app passwords require 2-Step Verification; create a dedicated app password and revoke it if no longer needed. Never commit secrets or paste them into issue text.

## What to expect

- The search runs 11 keyword searches and deduplicates listings by provider ID.
- Only listings with a valid posted date within the last 30 days and at least one resume-skill match are eligible.
- The email includes up to 20 roles, ranked by skills overlap, target-company name match (Walmart, Google, NxtWave), and recency.
- The number can be lower than 20 when the provider has fewer recent matches; the workflow reports this rather than inventing opportunities. It will fail clearly when credentials are missing or the provider has no current matching listings.
- These are aggregator listings with an apply-through URL. Check the destination and confirm the employer is still accepting applications. The workflow does not auto-submit applications or guarantee interview outcomes.

## Run once manually

Open **Actions → Daily JobHunter email → Run workflow** after adding the four secrets. Inspect the workflow logs for the count and email send result. Do not echo secret values in logs.
