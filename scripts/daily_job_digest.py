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

# Six Google Jobs searches per scheduled run stay below SerpApi's published
# 250-search monthly free allowance (manual workflow runs also consume quota).
SERPAPI_QUERIES = [
    "software engineer", "backend engineer", "backend developer",
    "Java developer", "Python developer", "site reliability engineer",
]

def get_json(url: str) -> dict:
    request = urllib.request.Request(url, headers={"Accept": "application/json", "User-Agent": "JobHunter-DailyDigest/1.0"})
    with urllib.request.urlopen(request, timeout=30) as response:
        return json.load(response)


def search_jobs() -> tuple[list[dict], int]:
    found: dict[str, dict] = {}

    def keep(raw: dict, source: str, key: str, title: str, company: str,
             location: str, url: str, description: str, posted: str | None,
             salary: str = "") -> None:
        if not url or not title:
            return
        item = {
            "id": key or url, "title": title, "company": company or "Unknown company",
            "location": location or "India", "url": url,
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
    for raw in found.values():
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
        # Focus on roles aligned with roughly three years' experience.
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
            "company": raw.get("company") or "Unknown company",
            "description": description,
            "location": raw.get("location") or "Remote",
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
    return jobs, len(found)


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


def parse_relative_date(value: str | None) -> dt.datetime | None:
    """Parse Google Jobs labels such as '3 days ago'; unknown dates stay unknown."""
    if not value:
        return None
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


def salary_text(job: dict) -> str:
    low, high = job.get("salary_min"), job.get("salary_max")
    currency = job.get("salary_currency", "")
    period = job.get("salary_period", "")
    if low and high:
        return f"{low:,.0f} - {high:,.0f} {currency} {period}".strip()
    if low:
        return f"From {low:,.0f} {currency} {period}".strip()
    return "Not listed"


def send_email(jobs: list[dict], total_fetched: int) -> None:
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
    message.set_content(plain_body(jobs, total_fetched, today))
    message.add_alternative(html_body(jobs, total_fetched, today), subtype="html")
    with smtplib.SMTP(host, port, timeout=30) as server:
        server.starttls()
        server.login(sender, password)
        server.send_message(message)


def plain_body(jobs: list[dict], total_fetched: int, today: str) -> str:
    lines = [f"Your JobHunter matches for {today}", "", f"Jobs fetched: {total_fetched}", f"Resume matches: {len(jobs)}", "Automatically applied: 0 (no supported employer submission integration)", "Applications reported failed: 0 (none attempted)", f"Manual follow-up required: {len(jobs)}", "Application success ratio: 0% — below 70%, so all matches are listed below for manual application.", ""]
    for i, job in enumerate(jobs, 1):
        lines.extend([f"{i}. {job['title']} — {job['company']}", f"   {job['location']} | posted {job['created']} | salary: {job['salary']}", f"   Resume fit: {', '.join(job['skills'])}", f"   Source: {job['source']}", f"   Listing: {job['url']}", ""])
    lines.append("Jobs are searched from India-wide Google Jobs results when SERPAPI_API_KEY is configured, plus India-eligible remote feeds. Search APIs return listings, not a general-purpose application submission service. No application was submitted; review each employer's questions, consent terms, and listing before applying.")
    return "\n".join(lines)


def html_body(jobs: list[dict], total_fetched: int, today: str) -> str:
    dashboard = ("<table role='presentation' style='width:100%;border-collapse:collapse;margin:18px 0'>"
                 "<tr>" + "".join(
                     f"<td style='background:#f1f5f9;padding:16px;border-radius:8px;text-align:center'><div style='font-size:24px;font-weight:bold'>{value}</div><div style='color:#475569'>{html.escape(label)}</div></td>"
                     for value, label in [(str(total_fetched), "Jobs fetched"), (str(len(jobs)), "Resume matches"), ("0", "Auto-applied"), (str(len(jobs)), "Manual follow-up")]
                 ) + "</tr></table><p><b>Application success ratio: 0%.</b> This is below your 70% threshold, so every matching job is included below for manual follow-up. No application was attempted or falsely counted as failed.</p>")
    cards = []
    for i, job in enumerate(jobs, 1):
        cards.append(f"<li><h3>{i}. {html.escape(job['title'])} — {html.escape(job['company'])}</h3><p>{html.escape(job['location'])} · Posted {html.escape(job['created'])} · Salary: {html.escape(job['salary'])}</p><p>Resume fit: {html.escape(', '.join(job['skills']))}</p><p>Source: {html.escape(job['source'])}</p><p><a href=\"{html.escape(job['url'], quote=True)}\">Open listing / application</a></p></li>")
    note = "India-wide Google Jobs search is enabled when SERPAPI_API_KEY is configured. No-key sources add India-eligible remote roles. Listings are ranked by resume fit, level, and freshness. Search providers do not provide a universal application submission service; review employer questions, consent terms, and current availability before applying."
    return f"<html><body style='font-family:Arial,sans-serif;color:#0f172a;max-width:850px;margin:auto;padding:20px'><h1>JobHunter daily dashboard — {html.escape(today)}</h1>{dashboard}<p style='color:#475569'>{html.escape(note)}</p><h2>Manual follow-up list</h2><ol>{''.join(cards)}</ol></body></html>"


def main() -> int:
    try:
        jobs, total_fetched = search_jobs()
        if not jobs:
            if not total_fetched:
                raise RuntimeError("No listings were returned by configured job sources; configure SERPAPI_API_KEY for India-wide Google Jobs search.")
            jobs = []
        send_email(jobs, total_fetched)
        print(f"Fetched {total_fetched}; resume matches {len(jobs)}; applications submitted 0; sent digest to {os.getenv('JOB_EMAIL_TO', 'gauravchaudhary.hck@gmail.com')}.")
        if len(jobs) < 20:
            print(f"Only {len(jobs)} eligible current listings were available today; no fictitious listings were added.")
        return 0
    except Exception as exc:
        print(f"JobHunter daily digest failed: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
