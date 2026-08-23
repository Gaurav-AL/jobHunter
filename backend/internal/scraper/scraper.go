package scraper

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"backend/internal/repository"
)

var linkPattern = regexp.MustCompile(`(?is)<a[^>]+href=["']([^"']+)["'][^>]*>(.*?)</a>`)
var tagPattern = regexp.MustCompile(`(?is)<[^>]+>`)
var spacePattern = regexp.MustCompile(`\s+`)

type SourceCandidate struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Status string `json:"status"`
}

// ScrapeJobs fetches a career page and extracts likely job links. It falls back to
// one source-level item when the page is reachable but does not expose parseable links.
func ScrapeJobs(sourceURL string) ([]*repository.Job, error) {
	log.Printf("Scraping source: %s", sourceURL)

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Get(sourceURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("source returned HTTP %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2_000_000))
	if err != nil {
		return nil, err
	}
	body := string(bodyBytes)
	sourceParsed, _ := url.Parse(sourceURL)

	seen := map[string]bool{}
	var jobs []*repository.Job
	for _, match := range linkPattern.FindAllStringSubmatch(body, -1) {
		href := strings.TrimSpace(match[1])
		title := cleanHTML(match[2])
		if href == "" || title == "" || !looksLikeJob(title, href) {
			continue
		}

		jobURL := resolveURL(sourceParsed, href)
		if jobURL == "" || seen[jobURL] {
			continue
		}
		seen[jobURL] = true
		jobs = append(jobs, &repository.Job{
			Title:       title,
			Description: fmt.Sprintf("Job opening discovered on %s. Review the source page for full details.", sourceURL),
			URL:         jobURL,
			ScrapedAt:   time.Now(),
		})
		if len(jobs) >= 20 {
			break
		}
	}

	if len(jobs) == 0 {
		jobs = append(jobs, &repository.Job{
			Title:       "Career page ready for review",
			Description: fmt.Sprintf("The source %s is reachable. No individual job links were detected yet; review the page manually or add a more specific jobs URL.", sourceURL),
			URL:         sourceURL,
			ScrapedAt:   time.Now(),
		})
	}

	return jobs, nil
}

// ScrapeJob keeps the old single-job API available for callers/tests.
func ScrapeJob(sourceURL string) (*repository.Job, error) {
	jobs, err := ScrapeJobs(sourceURL)
	if err != nil {
		return nil, err
	}
	return jobs[0], nil
}

func DiscoverSources(query string) []SourceCandidate {
	query = strings.TrimSpace(query)
	if query == "" {
		return defaultCandidates()
	}

	host := strings.ToLower(query)
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimPrefix(host, "www.")
	host = strings.TrimSuffix(host, "/")
	if !strings.Contains(host, ".") {
		host = host + ".com"
	}

	name := strings.TrimSuffix(strings.Split(host, ".")[0], "/")
	candidates := []SourceCandidate{
		{Name: strings.Title(name) + " Careers", URL: "https://" + host + "/careers"},
		{Name: strings.Title(name) + " Jobs", URL: "https://" + host + "/jobs"},
		{Name: strings.Title(name) + " Lever", URL: "https://jobs.lever.co/" + strings.Split(host, ".")[0]},
		{Name: strings.Title(name) + " Greenhouse", URL: "https://boards.greenhouse.io/" + strings.Split(host, ".")[0]},
	}

	for i := range candidates {
		candidates[i].Status = probe(candidates[i].URL)
	}
	return candidates
}

func defaultCandidates() []SourceCandidate {
	candidates := []SourceCandidate{
		{Name: "Google Careers", URL: "https://www.google.com/about/careers/applications/jobs/results/"},
		{Name: "Microsoft Careers", URL: "https://jobs.careers.microsoft.com/global/en/search"},
		{Name: "OpenAI Careers", URL: "https://openai.com/careers/search/"},
		{Name: "Stripe Jobs", URL: "https://stripe.com/jobs/search"},
		{Name: "Netflix Jobs", URL: "https://explore.jobs.netflix.net/careers"},
	}
	for i := range candidates {
		candidates[i].Status = probe(candidates[i].URL)
	}
	return candidates
}

func probe(candidateURL string) string {
	client := &http.Client{Timeout: 6 * time.Second}
	req, err := http.NewRequest(http.MethodHead, candidateURL, nil)
	if err != nil {
		return "invalid"
	}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode == http.StatusMethodNotAllowed {
		resp, err = client.Get(candidateURL)
	}
	if err != nil {
		return "unreachable"
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return "available"
	}
	return fmt.Sprintf("http_%d", resp.StatusCode)
}

func cleanHTML(value string) string {
	value = tagPattern.ReplaceAllString(value, " ")
	value = strings.ReplaceAll(value, "&amp;", "&")
	value = strings.ReplaceAll(value, "&nbsp;", " ")
	return strings.TrimSpace(spacePattern.ReplaceAllString(value, " "))
}

func looksLikeJob(title, href string) bool {
	value := strings.ToLower(title + " " + href)
	keywords := []string{"engineer", "developer", "manager", "designer", "analyst", "scientist", "intern", "remote", "apply", "job", "career", "opening", "role"}
	for _, keyword := range keywords {
		if strings.Contains(value, keyword) {
			return true
		}
	}
	return false
}

func resolveURL(base *url.URL, href string) string {
	parsed, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return base.ResolveReference(parsed).String()
}
