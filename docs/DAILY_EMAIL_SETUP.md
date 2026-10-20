# Daily resume-matched email digest

The GitHub Actions workflow runs daily at 08:47 India Standard Time and can also be started manually from the Actions tab. It searches Jobicy's public, no-key remote-job feed (general remote and APAC-eligible listings) for software engineering roles, ranks recent matches against the skills and experience represented in Gaurav Chaudhary's supplied resume, filters out clear senior/lead/management jobs, and emails up to 30 current matches to `gauravchaudhary.hck@gmail.com`.

## Required repository secrets

In **Gaurav-AL/jobHunter → Settings → Secrets and variables → Actions**, add:

- `SMTP_USER` — the Gmail account that sends the digest (can be the destination address)
- `SMTP_PASSWORD` — a Gmail App Password; do not use the normal account password

Job discovery uses Jobicy's public feed and requires no API key or credit card. The feed is remote-only, includes jobs published during the previous seven days, and links to Jobicy listings. It does not replace an India-wide local/on-site job index. The email sender must permit SMTP sending. If using Gmail, app passwords require 2-Step Verification; create a dedicated app password and revoke it if no longer needed. Never commit secrets or paste them into issue text.

## What to expect

- The cron makes two no-key feed requests (general remote and APAC) and deduplicates by Jobicy listing ID.
- Only listings with a valid posted date within the last seven days and at least one resume-skill match are eligible.
- The email includes up to 30 roles (so it can exceed 20 when enough good matches exist). No company receives a preferred-company bonus and no company is excluded; roles are scored by resume-skill fit, role level, and freshness, then spread across distinct employers where the search results allow it.
- The number can be lower than 20 when the provider has fewer recent matches; the workflow reports this rather than inventing opportunities. It will fail clearly when credentials are missing or the provider has no current matching listings.
- These are Jobicy listings; the public API preserves the Jobicy listing URL. Direct employer application URLs require Jobicy’s separate paid API. Check the destination and confirm the employer is still accepting applications. The workflow does not auto-submit applications or guarantee interview outcomes.

## Run once manually

Open **Actions → Daily JobHunter email → Run workflow** after adding the two SMTP secrets. Inspect the workflow logs for the count and email send result. Do not echo secret values in logs.
