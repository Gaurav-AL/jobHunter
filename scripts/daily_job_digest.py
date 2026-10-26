#!/usr/bin/env python3
"""Create and email a daily, resume-matched software engineering job digest."""
from __future__ import annotations

import datetime as dt
import html
import json
import os
import re
import smtplib
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from email.message import EmailMessage

PROFILE = {
    "years_experience": 2.2,
    "roles": [
        "software engineer", "backend engineer", "backend developer",
        "software development engineer", "associate software engineer",
        "software developer", "software engineer freelancer",
    ],
    "skills": [
        "java", "python", "go", "golang", "c++", "spring boot", "django",
        "flask", "gin", "fastapi", "docker", "kubernetes", "mysql", "grafana",
        "redash", "git", "tuxedo", "postman", "agentic ai", "linux", "aws", "gcp",
        "microservices", "distributed systems", "canary deployment", "observability",
        "integration testing", "database connection pooling", "production support", "concurrency",
    ],
}

# Six Google Jobs searches per scheduled run stay below SerpApi's published
# 250-search monthly free allowance (manual workflow runs also consume quota).
SERPAPI_QUERIES = [
    "software engineer", "backend engineer", "backend developer",
    "Java developer", "Python developer", "site reliability engineer",
]

SEEN_JOBS_FILE = os.getenv("SEEN_JOBS_FILE", ".cache/seen_jobs.json")


def load_seen_jobs() -> dict[str, str]:
    """Load previously sent job identifiers with their sent date."""
    if not os.path.exists(SEEN_JOBS_FILE):
        return {}
    try:
        with open(SEEN_JOBS_FILE, "r", encoding="utf-8") as f:
            data = json.load(f)
            if isinstance(data, dict):
                return data
            if isinstance(data, list):
                return {str(k): "" for k in data}
    except Exception as e:
        print(f"Notice: could not read seen jobs cache ({e}); starting fresh.", file=sys.stderr)
    return {}


def save_seen_jobs(seen: dict[str, str]) -> None:
    """Save seen job identifiers to disk, pruning records older than 21 days."""
    try:
        os.makedirs(os.path.dirname(os.path.abspath(SEEN_JOBS_FILE)), exist_ok=True)
        cutoff = (dt.datetime.now(dt.timezone.utc) - dt.timedelta(days=21)).isoformat()
        pruned = {k: v for k, v in seen.items() if not v or v >= cutoff}
        with open(SEEN_JOBS_FILE, "w", encoding="utf-8") as f:
            json.dump(pruned, f, indent=2)
    except Exception as e:
        print(f"Notice: could not save seen jobs cache ({e}).", file=sys.stderr)


def get_json(url: str) -> dict:
    request = urllib.request.Request(url, headers={"Accept": "application/json", "User-Agent": "JobHunter-DailyDigest/1.0"})
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.load(response)


def search_jobs(seen_jobs: dict[str, str] | None = None) -> tuple[list[dict], int, int]:
    found: dict[str, dict] = {}
    seen_map = seen_jobs or {}

    def keep(raw: dict, source: str, key: str, title: str, company: str,
             location: str, url: str, description: str, posted: str | None,
             salary: str = "") -> None:
        if not url or not title:
            return
        item = {
            "id": key or url, "title": title, "company": company or "Company not provided",
            "location": location or "Location not specified", "url": url,
            "description": description or "", "pubDate": posted,
            "salary_text": salary, "source": source, "raw": raw,
        }
        found[f"{source}:{item['id']}"] = item

    # India-wide Google Jobs search. SerpApi's free plan is capped at 250
    # successful searches monthly, so keep this to six queries per daily run.
    serpapi_key = os.getenv("SERPAPI_API_KEY", "").strip()
    if serpapi_key:
        for query in SERPAPI_QUERIES:
            params = urllib.parse.urlencode({
                "engine": "google_jobs", "q": query, "location": "India",
                "google_domain": "google.com", "gl": "in", "hl": "en",
                "api_key": serpapi_key,
            })
            try:
                payload = get_json("https://serpapi.com/search?" + params)
                for raw in payload.get("jobs_results", []):
                    extensions = raw.get("detected_extensions") or {}
                    applications = raw.get("apply_options") or []
                    apply_url = next((x.get("link") for x in applications if x.get("link")), "")
                    keep(raw, "Google Jobs via SerpApi", str(raw.get("job_id", "")),
                         raw.get("title", ""), raw.get("company_name", ""),
                         raw.get("location", "India"), apply_url or raw.get("share_link", ""),
                         raw.get("description", ""), extensions.get("posted_at"),
                         extensions.get("salary", ""))
            except Exception as exc:
                # Do not print the request URL: it contains the API key.
                detail = f"HTTP {exc.code}" if isinstance(exc, urllib.error.HTTPError) else type(exc).__name__
                print(f"SerpApi query '{query}' failed ({detail}).", file=sys.stderr)

    # No-key remote feeds add India-eligible remote positions.
    for geo in (None, "apac"):
        params = {"count": 200}
        if geo:
            params["geo"] = geo
        url = "https://jobicy.com/api/v2/remote-jobs?" + urllib.parse.urlencode(params)
        try:
            payload = get_json(url)
            for raw in payload.get("jobs", []):
                keep(raw, "Jobicy", str(raw.get("id", "")), raw.get("jobTitle", ""),
                     raw.get("companyName", ""), raw.get("jobGeo", "Remote"),
                     raw.get("url", ""), str(raw.get("jobDescription", "")) + " " + str(raw.get("jobExcerpt", "")),
                     raw.get("pubDate"), "")
        except Exception as exc:
            print(f"Jobicy feed failed: {exc}", file=sys.stderr)
        time.sleep(0.25)

    # Himalayas is a no-key remote feed with an India country filter.
    try:
        payload = get_json("https://himalayas.app/jobs/api/search?" + urllib.parse.urlencode({
            "country": "IN", "q": "software engineer", "sort": "recent", "page": 1,
        }))
        for raw in payload.get("jobs", []):
            keep(raw, "Himalayas", str(raw.get("id", "")), raw.get("title", ""),
                 raw.get("companyName", ""), raw.get("location", "India"),
                 raw.get("applicationLink") or raw.get("url", ""),
                 raw.get("description", ""), raw.get("pubDate") or raw.get("publishedAt"),
                 raw.get("salary", ""))
    except Exception as exc:
        print(f"Himalayas feed failed: {exc}", file=sys.stderr)

    # RemoteOK — worldwide remote, no API key required.
    # The first element of the array is a legal notice object, so we skip non-dict entries.
    try:
        items = get_json("https://remoteok.com/api")
        for raw in items:
            if not isinstance(raw, dict) or not raw.get("id"):
                continue
            job_url = raw.get("url") or (
                "https://remoteok.com/remote-jobs/" + raw["slug"] if raw.get("slug") else ""
            )
            tags = raw.get("tags") or []
            description = raw.get("description") or (", ".join(tags) if tags else "")
            keep(raw, "RemoteOK", str(raw.get("id", "")), raw.get("position", ""),
                 raw.get("company", ""), raw.get("location") or "Worldwide remote",
                 job_url, description, raw.get("date"), "")
    except Exception as exc:
        print(f"RemoteOK feed failed: {exc}", file=sys.stderr)

    # Arbeitnow — Europe/remote jobs, no API key required.
    try:
        payload = get_json("https://www.arbeitnow.com/api/job-board-api")
        for raw in payload.get("data", []):
            if not raw.get("slug") or not raw.get("title"):
                continue
            job_url = raw.get("url") or ("https://www.arbeitnow.com/jobs/" + raw["slug"])
            tags = raw.get("tags") or []
            description = raw.get("description") or (", ".join(tags) if tags else "")
            created_at = raw.get("created_at")
            posted = (
                dt.datetime.fromtimestamp(created_at, dt.timezone.utc).isoformat()
                if isinstance(created_at, (int, float)) else None
            )
            keep(raw, "Arbeitnow", str(raw.get("slug", "")), raw.get("title", ""),
                 raw.get("company_name", ""), raw.get("location") or ("Remote" if raw.get("remote") else ""),
                 job_url, description, posted, "")
    except Exception as exc:
        print(f"Arbeitnow feed failed: {exc}", file=sys.stderr)

    # Optional India-only feed. A free Jobvetta key can be stored as a GitHub secret.
    jobvetta_key = os.getenv("JOBVETTA_API_KEY", "").strip()
    if jobvetta_key:
        try:
            request = urllib.request.Request(
                "https://api.jobvetta.com/v1/jobs?" + urllib.parse.urlencode({"days": 7, "limit": 10}),
                headers={"Accept": "application/json", "Authorization": f"Bearer {jobvetta_key}",
                         "User-Agent": "JobHunter-DailyDigest/1.0"},
            )
            with urllib.request.urlopen(request, timeout=30) as response:
                payload = json.load(response)
            for raw in payload.get("jobs", []):
                posted = raw.get("created_at")
                if isinstance(posted, (int, float)):
                    posted = dt.datetime.fromtimestamp(posted, dt.timezone.utc).isoformat()
                keep(raw, "Jobvetta", str(raw.get("job_id", "")), raw.get("title", ""),
                     raw.get("company", ""), raw.get("location", "India"), raw.get("url", ""),
                     str(raw.get("description") or "") + " " + " ".join(raw.get("skills_required") or []), posted,
                     str(raw.get("salary_min") or ""))
        except Exception as exc:
            print(f"Jobvetta feed failed: {exc}", file=sys.stderr)

    cutoff = dt.datetime.now(dt.timezone.utc) - dt.timedelta(days=7)
    candidates = []
    skipped_previously_sent = 0
    for raw in found.values():
        job_url = str(raw.get("url", "")).strip()
        job_id = str(raw.get("id", "")).strip()
        src_id = f"{raw.get('source')}:{job_id}"

        # Exclude jobs already emailed in previous runs so only fresh ones are sent
        if job_url in seen_map or job_id in seen_map or src_id in seen_map:
            skipped_previously_sent += 1
            continue

        created = parse_date(raw.get("pubDate"))
        if created is None and raw.get("source") == "Google Jobs via SerpApi":
            created = parse_relative_date(raw.get("pubDate"))
        if created is None or created < cutoff:
            continue
        title = str(raw.get("title", ""))
        description = str(raw.get("description", ""))
        text = f"{title} {description}".lower()
        if not any(term in title.lower() for term in ("engineer", "developer", "sre", "programmer")):
            continue
        # Focus on roles aligned with the user's roughly 2.2 years' experience.
        if any(term in title.lower() for term in ("senior", "staff", "principal", "director", "engineering manager", "head of", "vice president", "vp ", "lead engineer")):
            continue
        skill_matches = [skill for skill in PROFILE["skills"] if re.search(r"(?<![a-z0-9])" + re.escape(skill) + r"(?![a-z0-9])", text)]
        if not skill_matches:
            continue
        title_lower = title.lower()
        role_matches = sum(role in title_lower for role in PROFILE["roles"])
        level = str((raw.get("raw") or {}).get("jobLevel", "")).lower()
        level_fit = 2 if any(term in title_lower or term in level for term in ("associate", "junior", "entry", "mid", "intermediate")) else 0
        age_days = max(0, (dt.datetime.now(dt.timezone.utc) - created).days)
        freshness = max(0, 2 - age_days // 4)
        job = {
            "id": raw.get("id"),
            "title": title,
            "company": raw.get("company") or "Company not provided",
            "description": description,
            "location": raw.get("location") or "Location not specified",
            "url": raw.get("url", ""),
            "created": created,
            "salary_min": (raw.get("raw") or {}).get("salaryMin"),
            "salary_max": (raw.get("raw") or {}).get("salaryMax"),
            "salary_currency": (raw.get("raw") or {}).get("salaryCurrency", ""),
            "salary_period": (raw.get("raw") or {}).get("salaryPeriod", ""),
            "salary_text": raw.get("salary_text", ""),
            "source": raw.get("source", "Unknown"),
        }
        score = len(skill_matches) + 2 * role_matches + level_fit + freshness
        candidates.append((score, created, job, skill_matches))
    candidates.sort(key=lambda item: (item[0], item[1]), reverse=True)
    jobs = [format_job(item) for item in diversify_by_company(candidates, limit=100)]
    return jobs, len(found), skipped_previously_sent


def diversify_by_company(candidates: list[tuple], limit: int) -> list[tuple]:
    """Preserve fit ranking while spreading the digest across distinct employers."""
    groups: dict[str, list[tuple]] = {}
    for item in candidates:
        employer = item[2].get("company", "Company not provided")
        key = " ".join(employer.casefold().split())
        groups.setdefault(key, []).append(item)
    # Prioritize employers whose best listing is the strongest match, then take one
    # listing per employer per pass until the digest is full.
    employers = sorted(groups, key=lambda key: (groups[key][0][0], groups[key][0][1]), reverse=True)
    selected = []
    round_index = 0
    while len(selected) < limit:
        added_this_round = False
        for employer in employers:
            if round_index < len(groups[employer]):
                selected.append(groups[employer][round_index])
                added_this_round = True
                if len(selected) == limit:
                    break
        if not added_this_round:
            break
        round_index += 1
    return selected


def parse_date(value: str | int | float | None) -> dt.datetime | None:
    if not value:
        return None
    if isinstance(value, (int, float)):
        try:
            # Some providers use Unix seconds; tolerate millisecond timestamps too.
            timestamp = value / 1000 if abs(value) >= 1_000_000_000_000 else value
            return dt.datetime.fromtimestamp(timestamp, dt.timezone.utc)
        except (OverflowError, OSError, ValueError):
            return None
    try:
        parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
        if parsed.tzinfo is None:
            parsed = parsed.replace(tzinfo=dt.timezone.utc)
        return parsed.astimezone(dt.timezone.utc)
    except (TypeError, ValueError):
        return None


def parse_relative_date(value: str | int | float | None) -> dt.datetime | None:
    """Parse Google Jobs labels such as '3 days ago'; unknown dates stay unknown."""
    if not value:
        return None
    if not isinstance(value, str):
        return parse_date(value)
    text = value.strip().lower()
    match = re.search(r"(today|just posted|yesterday|\d+\s+(?:minute|hour|day|week|month)s?\s+ago)", text)
    if not match:
        return parse_date(value)
    label = match.group(1)
    now = dt.datetime.now(dt.timezone.utc)
    if label in ("today", "just posted"):
        return now
    if label == "yesterday":
        return now - dt.timedelta(days=1)
    amount, unit = re.match(r"(\d+)\s+(minute|hour|day|week|month)", label).groups()
    count = int(amount)
    return now - dt.timedelta(**{
        "minute": {"minutes": count}, "hour": {"hours": count},
        "day": {"days": count}, "week": {"weeks": count},
        "month": {"days": count * 30},
    }[unit])


def format_job(item: tuple) -> dict:
    score, created, job, skills = item
    return {
        "title": job["title"],
        "company": job["company"],
        "location": job["location"],
        "created": created.strftime("%Y-%m-%d"),
        "url": job["url"],
        "skills": skills[:8],
        "salary": job.get("salary_text") or salary_text(job),
        "source": job["source"],
        "score": score,
    }


def company_recommendations(jobs: list[dict], limit: int = 8) -> list[dict]:
    """Suggest real employers in today's matches and link to ATS career-board discovery."""
    seen: set[str] = set()
    recommendations = []
    for job in jobs:
        company = str(job.get("company", "")).strip()
        key = " ".join(company.casefold().split())
        if not company or key in seen or key == "company not provided":
            continue
        seen.add(key)
        query = urllib.parse.urlencode({"q": f'{company} careers Greenhouse Lever Ashby jobs'})
        recommendations.append({"company": company, "url": "https://www.google.com/search?" + query})
        if len(recommendations) == limit:
            break
    return recommendations


def salary_text(job: dict) -> str:
    low, high = job.get("salary_min"), job.get("salary_max")
    currency = job.get("salary_currency", "")
    period = job.get("salary_period", "")
    def number(value: object) -> float | None:
        try:
            return float(str(value).replace(",", "").strip()) if value not in (None, "") else None
        except (TypeError, ValueError):
            return None
    low, high = number(low), number(high)
    if low and high:
        return f"{low:,.0f} - {high:,.0f} {currency} {period}".strip()
    if low:
        return f"From {low:,.0f} {currency} {period}".strip()
    return "Not listed"


def refresh_dashboard_sources() -> str:
    """Refresh saved ATS sources in the production database before composing the digest."""
    dashboard_url = os.getenv("JOBHUNTER_DASHBOARD_URL", "").strip().rstrip("/")
    request_url = os.getenv("ACTIONS_ID_TOKEN_REQUEST_URL", "").strip()
    request_token = os.getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "").strip()
    parsed = urllib.parse.urlparse(dashboard_url)
    if parsed.scheme != "https" or not parsed.netloc or not request_url or not request_token:
        return "not configured"
    try:
        token_request = urllib.request.Request(
            request_url + "&audience=" + urllib.parse.quote(dashboard_url + "/internal/scheduler/run", safe=""),
            headers={"Authorization": "Bearer " + request_token},
        )
        with urllib.request.urlopen(token_request, timeout=20) as response:
            token = json.loads(response.read(65536).decode("utf-8"))["value"]
        request = urllib.request.Request(
            dashboard_url + "/internal/scheduler/run", data=b"{}",
            headers={"Content-Type": "application/json", "Authorization": "Bearer " + token}, method="POST",
        )
        with urllib.request.urlopen(request, timeout=300) as response:
            data = json.loads(response.read(65536).decode("utf-8"))
        return str(data.get("message") or "completed")
    except Exception as exc:
        print(f"Saved-source refresh failed ({type(exc).__name__}); continuing with external job feeds.", file=sys.stderr)
        return "failed; see workflow logs"


def send_firebase_access_link() -> bool:
    """Ask Firebase Auth to email today's one-time sign-in link (session lasts two hours)."""
    api_key = os.getenv("FIREBASE_WEB_API_KEY", "").strip()
    dashboard_url = os.getenv("JOBHUNTER_DASHBOARD_URL", "").strip().rstrip("/")
    recipient = os.getenv("JOB_EMAIL_TO", "gauravchaudhary.hck@gmail.com").strip()
    parsed = urllib.parse.urlparse(dashboard_url)
    if not api_key or parsed.scheme != "https" or not parsed.netloc or parsed.username or parsed.password:
        print("Firebase access link not requested: set FIREBASE_WEB_API_KEY and a public HTTPS JOBHUNTER_DASHBOARD_URL.", file=sys.stderr)
        return False
    action = {
        "requestType": "EMAIL_SIGNIN",
        "email": recipient,
        # /auth/callback handles the link and redirects straight to the dashboard
        "continueUrl": dashboard_url + "/auth/callback",
        "canHandleCodeInApp": True,
    }
    request = urllib.request.Request(
        "https://identitytoolkit.googleapis.com/v1/accounts:sendOobCode?" + urllib.parse.urlencode({"key": api_key}),
        data=json.dumps(action).encode("utf-8"),
        headers={"Accept": "application/json", "Content-Type": "application/json", "Referer": dashboard_url + "/"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=20) as response:
            if response.status < 200 or response.status >= 300:
                raise RuntimeError(f"Firebase returned HTTP {response.status}")
        return True
    except urllib.error.HTTPError as exc:
        # Never print the request URL, because it contains the Firebase web API key.
        print(f"Firebase access link request failed (HTTP {exc.code}). Check Email link sign-in and the authorized domain.", file=sys.stderr)
    except Exception as exc:
        print(f"Firebase access link request failed ({type(exc).__name__}).", file=sys.stderr)
    return False


def send_email(jobs: list[dict], total_fetched: int, access_link_requested: bool, source_refresh: str = "not configured", skipped_previously_sent: int = 0) -> None:
    recipient = os.getenv("JOB_EMAIL_TO", "gauravchaudhary.hck@gmail.com").strip()
    sender = os.getenv("SMTP_USER", "").strip()
    password = os.getenv("SMTP_PASSWORD", "")
    host = os.getenv("SMTP_HOST", "smtp.gmail.com").strip()
    port = int(os.getenv("SMTP_PORT", "587"))
    if not sender or not password:
        raise RuntimeError("Configure SMTP_USER and SMTP_PASSWORD GitHub Actions secrets to enable email delivery.")
    today = dt.datetime.now(dt.timezone.utc).strftime("%d %b %Y")
    message = EmailMessage()
    message["Subject"] = f"JobHunter: {len(jobs)} fresh resume-matched software jobs — {today}"
    message["From"] = sender
    message["To"] = recipient
    dashboard_url = os.getenv("JOBHUNTER_DASHBOARD_URL", "").strip()
    parsed_dashboard_url = urllib.parse.urlparse(dashboard_url)
    if parsed_dashboard_url.scheme not in {"https", "http"} or not parsed_dashboard_url.netloc or parsed_dashboard_url.username or parsed_dashboard_url.password:
        dashboard_url = ""
    message.set_content(plain_body(jobs, total_fetched, today, dashboard_url, access_link_requested, skipped_previously_sent) + f"\nSaved JobHunter sources refresh: {source_refresh}\n")
    message.add_alternative(html_body(jobs, total_fetched, today, dashboard_url, access_link_requested, skipped_previously_sent), subtype="html")
    with smtplib.SMTP(host, port, timeout=30) as server:
        server.starttls()
        server.login(sender, password)
        server.send_message(message)


def plain_body(jobs: list[dict], total_fetched: int, today: str, dashboard_url: str = "", access_link_requested: bool = False, skipped_previously_sent: int = 0) -> str:
    lines = [
        f"Your JobHunter matches for {today}", "",
        f"Unique listings returned by feeds: {total_fetched}",
        f"Fresh new resume matches: {len(jobs)}",
        f"Previously emailed jobs filtered out: {skipped_previously_sent}",
        "Applications submitted/failed: Not tracked by the job digest",
        f"Manual follow-up required: {len(jobs)}",
        "Application success ratio: N/A (the digest does not submit applications).", ""
    ]
    if dashboard_url:
        lines.extend([f"Open your dashboard: {dashboard_url}"])
        if access_link_requested:
            lines.append("Firebase has sent a separate one-time sign-in email. Access lasts two hours after sign-in; enter this same email address when prompted.")
        else:
            lines.append("Temporary sign-in link was not sent today. Open the dashboard and request a link on the sign-in page.")
        lines.append("")
    recommendations = company_recommendations(jobs)
    if recommendations:
        lines.extend(["Companies to add from today's top matches:"])
        for recommendation in recommendations:
            lines.append(f"- {recommendation['company']}: {recommendation['url']} (find its public Greenhouse, Lever, or Ashby board, then add that board URL in JobHunter)")
        lines.append("")
    for i, job in enumerate(jobs, 1):
        lines.extend([f"{i}. {job['title']} — {job['company']}", f"   {job['location']} | posted {job['created']} | salary: {job['salary']}", f"   Resume fit: {', '.join(job['skills'])}", f"   Source: {job['source']}", f"   Listing: {job['url']}", ""])
    lines.append("Jobs are searched from India-wide Google Jobs results when SERPAPI_API_KEY is configured, plus India-eligible/APAC remote feeds. Previously emailed jobs are automatically excluded so you only receive new listings each morning and evening. Review each employer's questions and listing before applying.")
    return "\n".join(lines)


def html_body(jobs: list[dict], total_fetched: int, today: str, dashboard_url: str = "", access_link_requested: bool = False, skipped_previously_sent: int = 0) -> str:
    dashboard = ("<table role='presentation' style='width:100%;border-collapse:collapse;margin:18px 0'>"
                 "<tr>" + "".join(
                     f"<td style='background:#f1f5f9;padding:16px;border-radius:8px;text-align:center'><div style='font-size:24px;font-weight:bold'>{value}</div><div style='color:#475569'>{html.escape(label)}</div></td>"
                     for value, label in [(str(total_fetched), "Feed listings"), (str(len(jobs)), "Fresh new matches"), (str(skipped_previously_sent), "Prior jobs filtered"), (str(len(jobs)), "Manual follow-up")]
                 ) + "</tr></table><p><b>Applications submitted/failed: not tracked by this job digest.</b> Each morning and evening run filters out already emailed jobs to present only newly posted roles.</p>")
    cards = []
    for i, job in enumerate(jobs, 1):
        cards.append(f"<li><h3>{i}. {html.escape(job['title'])} — {html.escape(job['company'])}</h3><p>{html.escape(job['location'])} · Posted {html.escape(job['created'])} · Salary: {html.escape(job['salary'])}</p><p>Resume fit: {html.escape(', '.join(job['skills']))}</p><p>Source: {html.escape(job['source'])}</p><p><a href=\"{html.escape(job['url'], quote=True)}\">Open listing / application</a></p></li>")
    note = "India-wide Google Jobs search is enabled when SERPAPI_API_KEY is configured. No-key sources add India-eligible remote roles. Previously emailed jobs are automatically filtered out to ensure only fresh opportunities are delivered."
    dashboard_link = ""
    if dashboard_url:
        sign_in_note = ("Firebase sent a separate one-time sign-in email. Access lasts two hours after sign-in." if access_link_requested else "No temporary sign-in email was sent today; request one from the sign-in page.")
        dashboard_link = f"<p><a href=\"{html.escape(dashboard_url, quote=True)}\" style='display:inline-block;background:#4f46e5;color:white;padding:12px 18px;border-radius:8px;text-decoration:none'>Open JobHunter dashboard</a></p><p>{html.escape(sign_in_note)}</p>"
    recommendations = company_recommendations(jobs)
    company_section = ""
    if recommendations:
        company_cards = "".join(f"<li><b>{html.escape(item['company'])}</b> — <a href=\"{html.escape(item['url'], quote=True)}\">find the company's public Greenhouse, Lever, or Ashby career board</a>, then add that board URL in JobHunter.</li>" for item in recommendations)
        company_section = f"<h2>Companies to add from today's top matches</h2><ol>{company_cards}</ol>"
    return f"<html><body style='font-family:Arial,sans-serif;color:#0f172a;max-width:850px;margin:auto;padding:20px'><h1>JobHunter daily dashboard — {html.escape(today)}</h1>{dashboard_link}{dashboard}<p style='color:#475569'>{html.escape(note)}</p>{company_section}<h2>Manual follow-up list</h2><ol>{''.join(cards)}</ol></body></html>"


def main() -> int:
    try:
        source_refresh = refresh_dashboard_sources()
        print(f"Saved JobHunter sources refresh: {source_refresh}")
        seen_jobs = load_seen_jobs()
        print(f"Loaded {len(seen_jobs)} previously seen job keys from persistent cache.")
        jobs, total_fetched, skipped_previously_sent = search_jobs(seen_jobs)
        print(f"Search results: {total_fetched} total feed listings, {len(jobs)} fresh new matches, {skipped_previously_sent} previously sent jobs excluded.")
        
        # If all fresh jobs were filtered out because no new postings appeared yet,
        # fallback to top recent jobs so the digest email remains useful.
        if not jobs and total_fetched > 0 and skipped_previously_sent > 0:
            print("No brand new listings appeared since last run; retrieving top recent active listings as fallback.")
            jobs, _, _ = search_jobs(seen_jobs=None)

        if not jobs:
            if not total_fetched:
                raise RuntimeError("No listings were returned by configured job sources; configure SERPAPI_API_KEY for India-wide Google Jobs search.")
            jobs = []

        access_link_requested = send_firebase_access_link()
        send_email(jobs, total_fetched, access_link_requested, source_refresh, skipped_previously_sent)
        
        # Record newly sent jobs into persistent cache
        now_iso = dt.datetime.now(dt.timezone.utc).isoformat()
        for j in jobs:
            if j.get("url"):
                seen_jobs[j["url"]] = now_iso
            if j.get("id"):
                seen_jobs[str(j["id"])] = now_iso
                seen_jobs[f"{j.get('source', '')}:{j['id']}"] = now_iso
        save_seen_jobs(seen_jobs)
        print(f"Updated seen jobs cache: {len(seen_jobs)} total records stored.")

        print(f"Fetched {total_fetched} unique listings; {len(jobs)} fresh matches sent to {os.getenv('JOB_EMAIL_TO', 'gauravchaudhary.hck@gmail.com')}.")
        return 0
    except Exception as exc:
        print(f"JobHunter daily digest failed: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
