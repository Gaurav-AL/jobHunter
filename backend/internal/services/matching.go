package services

import (
	"regexp"
	"sort"
	"strings"

	"backend/internal/repository"
)

type MatchResult struct {
	Score  int
	Reason string
}

var splitTokenPattern = regexp.MustCompile(`[^a-z0-9+#.]+`)

var stopWords = map[string]bool{
	"and": true, "are": true, "for": true, "from": true, "have": true, "into": true,
	"job": true, "new": true, "our": true, "the": true, "this": true, "with": true,
	"work": true, "you": true, "your": true, "will": true, "role": true, "page": true,
	"career": true, "opening": true, "ready": true, "review": true,
}

func MatchJobToProfile(user repository.User, job repository.Job) MatchResult {
	profileText := strings.Join([]string{
		user.ResumeText,
		user.ProfileSummary,
		user.TargetRoles,
		user.Skills,
		user.Location,
	}, " ")
	jobText := job.Title + " " + job.Description + " " + job.URL

	profileTokens := weightedTokens(profileText)
	jobTokens := weightedTokens(jobText)
	if len(profileTokens) == 0 {
		return MatchResult{Score: 10, Reason: "Profile is incomplete; add or extract a resume to improve matching."}
	}

	score := 0
	var hits []string
	for token, weight := range profileTokens {
		if jobWeight, ok := jobTokens[token]; ok {
			score += weight + jobWeight
			hits = append(hits, token)
		}
	}
	// Prefer direct ATS boards as requested. Aggregator jobs remain eligible,
	// but rank below Greenhouse and the other direct company ATS sources.
	switch strings.ToLower(job.Provider) {
	case "greenhouse":
		score += 20
	case "lever", "ashby":
		score += 10
	case "himalayas":
		score -= 15
	case "jobicy":
		score -= 10
	}
	// Treat India as a preference, not a hard filter: include remote listings and
	// reduce explicit non-India matches while preserving other strong-fit roles.
	if prefersIndia(user.Location) {
		location := strings.ToLower(job.Location)
		switch {
		case strings.Contains(location, "india"):
			score += 18
		case containsAny(location, "bengaluru", "bangalore", "hyderabad", "pune", "mumbai", "chennai", "delhi", "noida", "gurugram", "gurgaon", "kolkata", "ahmedabad", "kochi", "coimbatore", "jaipur", "indore"):
			score += 16
		case location == "":
			// Missing location is unknown, so it receives no preference bonus.
		case strings.Contains(location, "remote") || strings.Contains(location, "anywhere"):
			score += 5
		default:
			score -= 8
		}
	}

	titleBoost := 0
	titleTokens := weightedTokens(job.Title)
	for token := range titleTokens {
		if profileTokens[token] > 0 {
			titleBoost += 8
		}
	}
	score += titleBoost

	if strings.Contains(strings.ToLower(user.TargetRoles), strings.ToLower(job.Title)) {
		score += 15
	}

	if score > 100 {
		score = 100
	}
	sort.Slice(hits, func(i, j int) bool {
		return profileTokens[hits[i]] > profileTokens[hits[j]]
	})
	if len(hits) > 6 {
		hits = hits[:6]
	}

	reason := "Matched from resume/profile keywords."
	if len(hits) > 0 {
		reason = "Matched on: " + strings.Join(hits, ", ") + "."
	}
	if score < 25 {
		reason = "Low resume/profile overlap. " + reason
	}

	return MatchResult{Score: score, Reason: reason}
}

func prefersIndia(location string) bool {
	value := strings.ToLower(location)
	return strings.TrimSpace(value) == "" || strings.Contains(value, "india") || strings.Contains(value, "bharat") ||
		containsAny(value, "bengaluru", "bangalore", "hyderabad", "pune", "mumbai", "chennai", "delhi", "noida", "gurugram", "gurgaon", "kolkata", "ahmedabad", "kochi", "coimbatore", "jaipur", "indore")
}

func containsAny(value string, options ...string) bool {
	for _, option := range options {
		if strings.Contains(value, option) {
			return true
		}
	}
	return false
}

func weightedTokens(value string) map[string]int {
	tokens := map[string]int{}
	for _, token := range splitTokenPattern.Split(strings.ToLower(value), -1) {
		token = strings.TrimSpace(token)
		if len(token) < 2 || stopWords[token] {
			continue
		}
		weight := 3
		if len(token) >= 5 {
			weight = 5
		}
		if isHighSignalSkill(token) {
			weight = 10
		}
		tokens[token] += weight
	}
	return tokens
}

func isHighSignalSkill(token string) bool {
	switch token {
	case "go", "golang", "vue", "react", "typescript", "javascript", "mysql", "postgres", "docker",
		"kubernetes", "aws", "gcp", "azure", "llm", "ai", "ml", "backend", "frontend", "fullstack",
		"distributed", "microservices", "grpc", "api", "rest", "python", "java", "node":
		return true
	default:
		return false
	}
}
