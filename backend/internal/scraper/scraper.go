package scraper

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"backend/internal/repository"
)

const (
	ProviderGreenhouse = "greenhouse"
	ProviderLever      = "lever"
	ProviderAshby      = "ashby"
)

var boardSlugPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,99}$`)
var tagPattern = regexp.MustCompile(`(?is)<[^>]+>`)
var spacePattern = regexp.MustCompile(`\s+`)

type SourceCandidate struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
}

// ParseBoardURL accepts public Greenhouse, Lever, and Ashby job board URLs and
// returns the provider, board slug, and canonical public board URL.
func ParseBoardURL(raw string) (string, string, string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return "", "", "", fmt.Errorf("enter a valid public job board URL")
	}
	host := strings.ToLower(u.Hostname())
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "", "", "", fmt.Errorf("the URL must include the company's job board name")
	}
	provider := ""
	switch {
	case host == "boards.greenhouse.io" || host == "job-boards.greenhouse.io":
		provider = ProviderGreenhouse
	case host == "jobs.lever.co" || host == "jobs.eu.lever.co":
		provider = ProviderLever
	case host == "jobs.ashbyhq.com":
		provider = ProviderAshby
	default:
		return "", "", "", fmt.Errorf("unsupported source; use a public Greenhouse, Lever, or Ashby job-board URL")
	}
	slug := parts[0]
	if !boardSlugPattern.MatchString(slug) {
		return "", "", "", fmt.Errorf("invalid job board name in URL")
	}
	canonicalHost := map[string]string{
		ProviderGreenhouse: "https://boards.greenhouse.io/",
		ProviderLever:      "https://jobs.lever.co/",
		ProviderAshby:      "https://jobs.ashbyhq.com/",
	}[provider]
	return provider, slug, canonicalHost + url.PathEscape(slug), nil
}

// ScrapeJobs fetches current, publicly listed jobs from an official ATS board API.
// It deliberately returns an empty result instead of inventing a placeholder job.
func ScrapeJobs(sourceURL string) ([]*repository.Job, error) {
	provider, slug, _, err := ParseBoardURL(sourceURL)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	var endpoint string
	switch provider {
	case ProviderGreenhouse:
		endpoint = "https://boards-api.greenhouse.io/v1/boards/" + url.PathEscape(slug) + "/jobs?content=true"
	case ProviderLever:
		host := "https://api.lever.co"
		if strings.Contains(strings.ToLower(sourceURL), "jobs.eu.lever.co") {
			host = "https://api.eu.lever.co"
		}
		endpoint = host + "/v0/postings/" + url.PathEscape(slug) + "?mode=json"
	case ProviderAshby:
		endpoint = "https://api.ashbyhq.com/posting-api/job-board/" + url.PathEscape(slug)
	}
	request, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "JobHunter/1.0 (+public ATS listings)")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s job board request failed: %w", provider, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s job board returned HTTP %d", provider, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	switch provider {
	case ProviderGreenhouse:
		var payload struct {
			Jobs []struct {
				ID          int64     `json:"id"`
				Title       string    `json:"title"`
				UpdatedAt   time.Time `json:"updated_at"`
				AbsoluteURL string    `json:"absolute_url"`
				Content     string    `json:"content"`
				Location    struct {
					Name string `json:"name"`
				} `json:"location"`
			} `json:"jobs"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("invalid Greenhouse job response: %w", err)
		}
		jobs := make([]*repository.Job, 0, len(payload.Jobs))
		for _, item := range payload.Jobs {
			if item.ID == 0 || item.Title == "" || item.AbsoluteURL == "" {
				continue
			}
			var publishedAt *time.Time
			if !item.UpdatedAt.IsZero() {
				published := item.UpdatedAt.UTC()
				publishedAt = &published
			}
			jobs = append(jobs, &repository.Job{ExternalID: fmt.Sprint(item.ID), Title: item.Title, Description: cleanHTML(item.Content), URL: item.AbsoluteURL, ApplyURL: item.AbsoluteURL, Location: item.Location.Name, PublishedAt: publishedAt, ScrapedAt: now, LastSeenAt: now, IsActive: true})
		}
		return jobs, nil
	case ProviderLever:
		var payload []struct {
			ID          string `json:"id"`
			Text        string `json:"text"`
			CreatedAt   int64  `json:"createdAt"`
			HostedURL   string `json:"hostedUrl"`
			ApplyURL    string `json:"applyUrl"`
			Description string `json:"descriptionPlain"`
			Categories  struct {
				Location string `json:"location"`
			} `json:"categories"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("invalid Lever job response: %w", err)
		}
		jobs := make([]*repository.Job, 0, len(payload))
		for _, item := range payload {
			if item.ID == "" || item.Text == "" || item.HostedURL == "" {
				continue
			}
			var publishedAt *time.Time
			if item.CreatedAt > 0 {
				published := time.UnixMilli(item.CreatedAt).UTC()
				publishedAt = &published
			}
			applyURL := item.ApplyURL
			if applyURL == "" {
				applyURL = item.HostedURL
			}
			jobs = append(jobs, &repository.Job{ExternalID: item.ID, Title: item.Text, Description: item.Description, URL: item.HostedURL, ApplyURL: applyURL, Location: item.Categories.Location, PublishedAt: publishedAt, ScrapedAt: now, LastSeenAt: now, IsActive: true})
		}
		return jobs, nil
	case ProviderAshby:
		var payload struct {
			Jobs []struct {
				Title       string    `json:"title"`
				Location    string    `json:"location"`
				Description string    `json:"descriptionPlain"`
				JobURL      string    `json:"jobUrl"`
				ApplyURL    string    `json:"applyUrl"`
				PublishedAt time.Time `json:"publishedAt"`
				IsListed    bool      `json:"isListed"`
				IsRemote    bool      `json:"isRemote"`
			} `json:"jobs"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("invalid Ashby job response: %w", err)
		}
		jobs := make([]*repository.Job, 0, len(payload.Jobs))
		for _, item := range payload.Jobs {
			if item.Title == "" || item.JobURL == "" || !item.IsListed {
				continue
			}
			applyURL := item.ApplyURL
			if applyURL == "" {
				applyURL = item.JobURL
			}
			var publishedAt *time.Time
			if !item.PublishedAt.IsZero() {
				published := item.PublishedAt.UTC()
				publishedAt = &published
			}
			jobs = append(jobs, &repository.Job{ExternalID: item.JobURL, Title: item.Title, Description: item.Description, URL: item.JobURL, ApplyURL: applyURL, Location: item.Location, PublishedAt: publishedAt, ScrapedAt: now, LastSeenAt: now, IsActive: true})
		}
		return jobs, nil
	}
	return nil, fmt.Errorf("unsupported provider %q", provider)
}

// ScrapeJob remains for callers that expect exactly one result.
func ScrapeJob(sourceURL string) (*repository.Job, error) {
	jobs, err := ScrapeJobs(sourceURL)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, fmt.Errorf("no currently published jobs found")
	}
	return jobs[0], nil
}

func DiscoverSources(query string) []SourceCandidate {
	query = strings.TrimSpace(query)
	if query == "" {
		return []SourceCandidate{{Name: "Paste a public ATS job-board URL", Status: "supported: Greenhouse, Lever, Ashby"}}
	}
	if !strings.Contains(query, "://") {
		query = "https://" + strings.TrimPrefix(query, "www.")
	}
	provider, slug, canonicalURL, err := ParseBoardURL(query)
	if err == nil {
		count, status := probeBoard(provider, slug, query)
		name := map[string]string{ProviderGreenhouse: "Greenhouse", ProviderLever: "Lever", ProviderAshby: "Ashby"}[provider]
		if count > 0 {
			status = fmt.Sprintf("verified · %d published jobs", count)
		}
		return []SourceCandidate{{Name: name + " · " + slug, URL: canonicalURL, Provider: provider, Status: status}}
	}
	return []SourceCandidate{{Name: "Unsupported career URL", URL: query, Status: "Paste the company's public Greenhouse, Lever, or Ashby board URL."}}
}

func probeBoard(provider, slug, originalURL string) (int, string) {
	client := &http.Client{Timeout: 8 * time.Second}
	var endpoint string
	switch provider {
	case ProviderGreenhouse:
		endpoint = "https://boards-api.greenhouse.io/v1/boards/" + url.PathEscape(slug) + "/jobs"
	case ProviderLever:
		host := "https://api.lever.co"
		if strings.Contains(strings.ToLower(originalURL), "jobs.eu.lever.co") {
			host = "https://api.eu.lever.co"
		}
		endpoint = host + "/v0/postings/" + url.PathEscape(slug) + "?mode=json"
	case ProviderAshby:
		endpoint = "https://api.ashbyhq.com/posting-api/job-board/" + url.PathEscape(slug)
	}
	request, _ := http.NewRequest(http.MethodGet, endpoint, nil)
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return 0, "unreachable"
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, fmt.Sprintf("board returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return 0, "invalid response"
	}
	count := 0
	switch provider {
	case ProviderLever:
		var jobs []json.RawMessage
		if json.Unmarshal(body, &jobs) == nil {
			count = len(jobs)
		}
	case ProviderGreenhouse:
		var payload struct {
			Jobs []json.RawMessage `json:"jobs"`
		}
		if json.Unmarshal(body, &payload) == nil {
			count = len(payload.Jobs)
		}
	case ProviderAshby:
		var payload struct {
			Jobs []json.RawMessage `json:"jobs"`
		}
		if json.Unmarshal(body, &payload) == nil {
			count = len(payload.Jobs)
		}
	}
	return count, "available, no published jobs"
}

func cleanHTML(value string) string {
	value = tagPattern.ReplaceAllString(value, " ")
	value = strings.NewReplacer("&amp;", "&", "&nbsp;", " ", "&lt;", "<", "&gt;", ">", "&quot;", "\"", "&#39;", "'").Replace(value)
	return strings.TrimSpace(spacePattern.ReplaceAllString(value, " "))
}
