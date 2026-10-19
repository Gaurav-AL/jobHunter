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
    "years_experience": 3,
    "roles": [
        "software engineer", "backend engineer", "backend developer",
        "software development engineer", "platform engineer", "site reliability engineer",
        "distributed systems engineer", "associate software engineer",
    ],
    "skills": [
        "java", "python", "go", "golang", "c++", "cobol", "spring boot",
        "django", "flask", "gin", "fastapi", "docker", "kubernetes", "mysql",
        "grafana", "redash", "linux", "aws", "gcp", "microservices",
        "distributed systems", "canary deployment", "observability", "integration testing",
        "database connection pooling", "production support", "concurrency",
    ],
    "location": os.getenv("JOB_LOCATION", "India"),
    "country": os.getenv("ADZUNA_COUNTRY", "in").lower(),
}
QUERIES = [
    "backend engineer java", "software engineer backend", "golang developer",
    "python backend developer", "software development engineer",
    "platform engineer kubernetes", "site reliability engineer", "distributed systems engineer",
    "cloud engineer", "microservices engineer",
]


def get_json(url: str) -> dict:
    request = urllib.request.Request(url, headers={"Accept": "application/json", "User-Agent": "JobHunter-DailyDigest/1.0"})
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.load(response)


def search_jobs() -> list[dict]:
    app_id, app_key = os.getenv("ADZUNA_APP_ID", "").strip(), os.getenv("ADZUNA_APP_KEY", "").strip()
    if not app_id or not app_key:
        raise RuntimeError("Configure ADZUNA_APP_ID and ADZUNA_APP_KEY GitHub Actions secrets (Adzuna requires both).")
    if not PROFILE["country"].isalpha() or len(PROFILE["country"]) != 2:
        raise RuntimeError("ADZUNA_COUNTRY must be a two-letter Adzuna country code.")

    found: dict[str, dict] = {}
    for query in QUERIES:
        params = urllib.parse.urlencode({
            "app_id": app_id, "app_key": app_key, "results_per_page": 50,
            "what": query, "where": PROFILE["location"], "sort_by": "date",
            "content-type": "application/json",
        })
        url = f"https://api.adzuna.com/v1/api/jobs/{PROFILE['country']}/search/1?{params}"
        try:
            results = get_json(url).get("results", [])
        except urllib.error.HTTPError as exc:
            message = exc.read(1024).decode("utf-8", "replace")
            raise RuntimeError(f"Adzuna request failed (HTTP {exc.code}): {message}") from exc
        for job in results:
            key = str(job.get("id") or job.get("redirect_url") or "")
            if key:
                found[key] = job
        time.sleep(0.2)

    cutoff = dt.datetime.now(dt.timezone.utc) - dt.timedelta(days=30)
    candidates = []
    for job in found.values():
        created = parse_date(job.get("created"))
        if created is None or created < cutoff:
            continue
        title = str(job.get("title", ""))
        description = str(job.get("description", ""))
        company_obj = job.get("company") or {}
        text = f"{title} {description}".lower()
        if not any(term in title.lower() for term in ("engineer", "developer", "sre", "programmer")):
            continue
        # Exclude clear senior/lead/management postings; preserve plausible associate/mid-level roles.
        if any(term in title.lower() for term in ("staff", "principal", "director", "engineering manager", "head of", "vice president", "vp ")):
            continue
        skill_matches = [skill for skill in PROFILE["skills"] if re.search(r"(?<![a-z0-9])" + re.escape(skill) + r"(?![a-z0-9])", text)]
        if len(skill_matches) < 1:
            continue
        title_lower = title.lower()
        role_matches = sum(role in title_lower for role in PROFILE["roles"])
        level_fit = 2 if any(term in title_lower for term in ("associate", "junior", "entry level", "mid-level", "sde i", "sde 1", "engineer ii")) else 0
        age_days = max(0, (dt.datetime.now(dt.timezone.utc) - created).days)
        freshness = max(0, 2 - age_days // 15)
        score = len(skill_matches) + 2 * role_matches + level_fit + freshness
        candidates.append((score, created, job, skill_matches))
    candidates.sort(key=lambda item: (item[0], item[1]), reverse=True)
    return [format_job(item) for item in diversify_by_company(candidates, limit=30)]


def diversify_by_company(candidates: list[tuple], limit: int) -> list[tuple]:
    """Preserve fit ranking while spreading the digest across distinct employers."""
    groups: dict[str, list[tuple]] = {}
    for item in candidates:
        employer = (item[2].get("company") or {}).get("display_name", "Unknown company")
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


def parse_date(value: str | None) -> dt.datetime | None:
    if not value:
        return None
    try:
        parsed = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
        if parsed.tzinfo is None:
            parsed = parsed.replace(tzinfo=dt.timezone.utc)
        return parsed.astimezone(dt.timezone.utc)
    except (TypeError, ValueError):
        return None


def format_job(item: tuple) -> dict:
    score, created, job, skills = item
    location = (job.get("location") or {}).get("display_name", PROFILE["location"])
    return {
        "title": job.get("title", "Software role"),
        "company": (job.get("company") or {}).get("display_name", "Unknown company"),
        "location": location,
        "created": created.strftime("%Y-%m-%d"),
        "url": job.get("redirect_url", ""),
        "skills": skills[:8],
        "salary": salary_text(job),
        "score": score,
    }


def salary_text(job: dict) -> str:
    low, high = job.get("salary_min"), job.get("salary_max")
    if low and high:
        return f"{low:,.0f} - {high:,.0f} (as listed)"
    if low:
        return f"From {low:,.0f} (as listed)"
    return "Not listed"


def send_email(jobs: list[dict]) -> None:
    recipient = os.getenv("JOB_EMAIL_TO", "gauravchaudhary.hck@gmail.com").strip()
    sender = os.getenv("SMTP_USER", "").strip()
    password = os.getenv("SMTP_PASSWORD", "")
    host = os.getenv("SMTP_HOST", "smtp.gmail.com").strip()
    port = int(os.getenv("SMTP_PORT", "587"))
    if not sender or not password:
        raise RuntimeError("Configure SMTP_USER and SMTP_PASSWORD GitHub Actions secrets to enable email delivery.")
    today = dt.datetime.now(dt.timezone.utc).strftime("%d %b %Y")
    message = EmailMessage()
    message["Subject"] = f"JobHunter: {len(jobs)} resume-matched software jobs — {today}"
    message["From"] = sender
    message["To"] = recipient
    message.set_content(plain_body(jobs, today))
    message.add_alternative(html_body(jobs, today), subtype="html")
    with smtplib.SMTP(host, port, timeout=30) as server:
        server.starttls()
        server.login(sender, password)
        server.send_message(message)


def plain_body(jobs: list[dict], today: str) -> str:
    lines = [f"Your JobHunter matches for {today}", "", f"Found {len(jobs)} current matches from the last 30 days.", ""]
    for i, job in enumerate(jobs, 1):
        lines.extend([f"{i}. {job['title']} — {job['company']}", f"   {job['location']} | posted {job['created']} | salary: {job['salary']}", f"   Resume fit: {', '.join(job['skills'])}", f"   Apply: {job['url']}", ""])
    lines.append("Matches are ranked by resume-skill fit, role level, and freshness, with results spread across distinct employers where possible. Review each listing and tailor your application before submitting. JobHunter does not submit applications for you.")
    return "\n".join(lines)


def html_body(jobs: list[dict], today: str) -> str:
    cards = []
    for i, job in enumerate(jobs, 1):
        cards.append(f"<li><h3>{i}. {html.escape(job['title'])} — {html.escape(job['company'])}</h3><p>{html.escape(job['location'])} · Posted {html.escape(job['created'])} · Salary: {html.escape(job['salary'])}</p><p>Resume fit: {html.escape(', '.join(job['skills']))}</p><p><a href=\"{html.escape(job['url'], quote=True)}\">Open official listing</a></p></li>")
    note = "Employers are not preferred or excluded. The list is ranked by resume-skill fit and freshness, then spread across employers where enough distinct companies have matching listings."
    return f"<html><body><h1>JobHunter matches — {html.escape(today)}</h1><p>{len(jobs)} recent roles matching your resume preferences.</p><p>{html.escape(note)}</p><ol>{''.join(cards)}</ol><p>Listings are from Adzuna’s job ad search and may redirect to a company or job-board page. Confirm that the employer is still accepting applications. JobHunter does not submit applications.</p></body></html>"


def main() -> int:
    try:
        jobs = search_jobs()
        if not jobs:
            raise RuntimeError("No recent resume-matched listings were returned; refusing to send an empty or invented job list.")
        send_email(jobs)
        print(f"Sent {len(jobs)} current matches to {os.getenv('JOB_EMAIL_TO', 'gauravchaudhary.hck@gmail.com')}.")
        if len(jobs) < 20:
            print(f"Only {len(jobs)} eligible current listings were available today; no fictitious listings were added.")
        return 0
    except Exception as exc:
        print(f"JobHunter daily digest failed: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
