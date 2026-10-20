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
}

def get_json(url: str) -> dict:
    request = urllib.request.Request(url, headers={"Accept": "application/json", "User-Agent": "JobHunter-DailyDigest/1.0"})
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.load(response)


def search_jobs() -> list[dict]:
    # Jobicy's public feed needs no account, API key, or paid tier. The unfiltered
    # feed gives broad remote coverage; APAC adds region-eligible opportunities.
    found: dict[str, dict] = {}
    for geo in (None, "apac"):
        params = {"count": 200}
        if geo:
            params["geo"] = geo
        url = "https://jobicy.com/api/v2/remote-jobs?" + urllib.parse.urlencode(params)
        try:
            payload = get_json(url)
        except urllib.error.HTTPError as exc:
            message = exc.read(1024).decode("utf-8", "replace")
            raise RuntimeError(f"Jobicy public jobs feed failed (HTTP {exc.code}): {message}") from exc
        if payload.get("success") is False:
            raise RuntimeError(f"Jobicy public jobs feed reported an error: {payload.get('error', 'unknown error')}")
        for job in payload.get("jobs", []):
            key = str(job.get("id") or job.get("url") or "")
            if key:
                found[key] = job
        time.sleep(0.25)

    cutoff = dt.datetime.now(dt.timezone.utc) - dt.timedelta(days=7)
    candidates = []
    for raw in found.values():
        created = parse_date(raw.get("pubDate"))
        if created is None or created < cutoff:
            continue
        title = str(raw.get("jobTitle", ""))
        description = str(raw.get("jobDescription", "")) + " " + str(raw.get("jobExcerpt", ""))
        text = f"{title} {description}".lower()
        if not any(term in title.lower() for term in ("engineer", "developer", "sre", "programmer")):
            continue
        # Focus on roles aligned with roughly three years' experience.
        if any(term in title.lower() for term in ("senior", "staff", "principal", "director", "engineering manager", "head of", "vice president", "vp ", "lead engineer")):
            continue
        skill_matches = [skill for skill in PROFILE["skills"] if re.search(r"(?<![a-z0-9])" + re.escape(skill) + r"(?![a-z0-9])", text)]
        if not skill_matches:
            continue
        title_lower = title.lower()
        role_matches = sum(role in title_lower for role in PROFILE["roles"])
        level = str(raw.get("jobLevel", "")).lower()
        level_fit = 2 if any(term in title_lower or term in level for term in ("associate", "junior", "entry", "mid", "intermediate")) else 0
        age_days = max(0, (dt.datetime.now(dt.timezone.utc) - created).days)
        freshness = max(0, 2 - age_days // 4)
        job = {
            "id": raw.get("id"),
            "title": title,
            "company": raw.get("companyName") or "Unknown company",
            "description": description,
            "location": raw.get("jobGeo") or "Remote",
            "url": raw.get("url", ""),
            "created": created,
            "salary_min": raw.get("salaryMin"),
            "salary_max": raw.get("salaryMax"),
            "salary_currency": raw.get("salaryCurrency", ""),
            "salary_period": raw.get("salaryPeriod", ""),
            "source": "Jobicy",
        }
        score = len(skill_matches) + 2 * role_matches + level_fit + freshness
        candidates.append((score, created, job, skill_matches))
    candidates.sort(key=lambda item: (item[0], item[1]), reverse=True)
    return [format_job(item) for item in diversify_by_company(candidates, limit=30)]


def diversify_by_company(candidates: list[tuple], limit: int) -> list[tuple]:
    """Preserve fit ranking while spreading the digest across distinct employers."""
    groups: dict[str, list[tuple]] = {}
    for item in candidates:
        employer = item[2].get("company", "Unknown company")
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
    return {
        "title": job["title"],
        "company": job["company"],
        "location": job["location"],
        "created": created.strftime("%Y-%m-%d"),
        "url": job["url"],
        "skills": skills[:8],
        "salary": salary_text(job),
        "source": job["source"],
        "score": score,
    }


def salary_text(job: dict) -> str:
    low, high = job.get("salary_min"), job.get("salary_max")
    currency = job.get("salary_currency", "")
    period = job.get("salary_period", "")
    if low and high:
        return f"{low:,.0f} - {high:,.0f} {currency} {period}".strip()
    if low:
        return f"From {low:,.0f} {currency} {period}".strip()
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
    lines = [f"Your JobHunter matches for {today}", "", f"Found {len(jobs)} current matches from the last seven days.", ""]
    for i, job in enumerate(jobs, 1):
        lines.extend([f"{i}. {job['title']} — {job['company']}", f"   {job['location']} | posted {job['created']} | salary: {job['salary']}", f"   Resume fit: {', '.join(job['skills'])}", f"   Source: {job['source']}", f"   Listing: {job['url']}", ""])
    lines.append("No employers are preferred or excluded. Matches are ranked by resume fit, role level, and freshness, then spread across employers where possible. This free public feed covers remote listings published in the last seven days. Review each Jobicy listing before applying. JobHunter does not submit applications for you.")
    return "\n".join(lines)


def html_body(jobs: list[dict], today: str) -> str:
    cards = []
    for i, job in enumerate(jobs, 1):
        cards.append(f"<li><h3>{i}. {html.escape(job['title'])} — {html.escape(job['company'])}</h3><p>{html.escape(job['location'])} · Posted {html.escape(job['created'])} · Salary: {html.escape(job['salary'])}</p><p>Resume fit: {html.escape(', '.join(job['skills']))}</p><p>Source: {html.escape(job['source'])}</p><p><a href=\"{html.escape(job['url'], quote=True)}\">Open listing on Jobicy</a></p></li>")
    note = "No employers are preferred or excluded. Roles are ranked by resume fit, role level, and freshness, then spread across employers where possible. The free public Jobicy feed covers remote roles and only exposes jobs from its current seven-day window."
    return f"<html><body><h1>JobHunter matches — {html.escape(today)}</h1><p>{len(jobs)} recent roles matching your resume preferences.</p><p>{html.escape(note)}</p><ol>{''.join(cards)}</ol><p>Jobicy is the source of these listings. Open the listing there and confirm that the employer is still accepting applications. JobHunter does not submit applications.</p></body></html>"


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
