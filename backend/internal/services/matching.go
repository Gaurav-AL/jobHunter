package services

import (
	"regexp"
	"strings"

	"backend/internal/repository"
)

type MatchResult struct {
	Score         int
	Reason        string
	MatchedSkills []string
}

// RelevantSkills returns the candidate's stated skills that are explicitly
// present in the job listing. This is more useful than generic token overlap.
func RelevantSkills(user repository.User, job repository.Job) []string {
	jobText := normalizedPhraseText(job.Title + " " + job.Description)
	profileSkills := strings.Split(strings.ReplaceAll(strings.ReplaceAll(user.Skills, "\n", ","), ";", ","), ",")
	seen := map[string]bool{}
	var hits []string
	for _, item := range profileSkills {
		skill := strings.TrimSpace(item)
		if len(skill) < 2 {
			continue
		}
		needle := normalizedPhraseText(skill)
		if strings.Contains(jobText, needle) && !seen[strings.ToLower(skill)] {
			seen[strings.ToLower(skill)] = true
			hits = append(hits, skill)
		}
	}
	return hits
}

var skillCatalog = []string{"Go", "Golang", "Java", "Python", "JavaScript", "TypeScript", "C++", "C#", "React", "Vue", "Angular", "Node.js", "Gin", "Django", "Flask", "Spring Boot", "FastAPI", "REST API", "GraphQL", "gRPC", "Microservices", "Distributed Systems", "MySQL", "PostgreSQL", "MongoDB", "Redis", "Docker", "Kubernetes", "AWS", "GCP", "Azure", "Linux", "Git", "CI/CD", "Jenkins", "Kafka", "RabbitMQ", "Terraform", "Machine Learning", "LLM", "Agentic AI", "HTML", "CSS", "SQL"}

func JobSkillsAsked(job repository.Job) []string {
	text := normalizedPhraseText(job.Title + " " + job.Description)
	seen := map[string]bool{}
	var skills []string
	for _, skill := range skillCatalog {
		needle := normalizedPhraseText(skill)
		if strings.Contains(text, needle) && !seen[strings.ToLower(skill)] {
			seen[strings.ToLower(skill)] = true
			skills = append(skills, skill)
		}
	}
	return skills
}

func normalizedPhraseText(value string) string {
	return " " + strings.Join(splitTokenPattern.Split(strings.ToLower(value), -1), " ") + " "
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
	profileTokens := weightedTokens(profileText)
	if len(profileTokens) == 0 {
		return MatchResult{Score: 10, Reason: "Profile is incomplete; add or extract a resume to improve matching."}
	}
	profileSkills := RelevantSkills(user, repository.Job{Title: profileText, Description: profileText})
	if len(profileSkills) == 0 {
		profileSkills = catalogSkillsInText(profileText)
	}
	jobSkills := JobSkillsAsked(job)
	matchedSkills := make([]string, 0, len(jobSkills))
	for _, skill := range jobSkills {
		for _, candidateSkill := range profileSkills {
			if strings.EqualFold(skill, candidateSkill) {
				matchedSkills = append(matchedSkills, skill)
				break
			}
		}
	}
	skillFit := 0
	if len(jobSkills) > 0 {
		skillFit = len(matchedSkills) * 65 / len(jobSkills)
	}
	if len(profileSkills) == 0 {
		skillFit = 0
	}

	// Role overlap is measured against the title so long resumes do not
	// overwhelm the score and drive every result to 100%.
	roleBasis := user.TargetRoles
	if strings.TrimSpace(roleBasis) == "" {
		roleBasis = user.ProfileSummary
	}
	titleTokens := weightedTokens(job.Title)
	roleTokens := weightedTokens(roleBasis)
	roleHits := 0
	for token := range titleTokens {
		if roleTokens[token] > 0 {
			roleHits++
		}
	}
	roleFit := 0
	if len(titleTokens) > 0 {
		roleFit = roleHits * 25 / len(titleTokens)
	}
	if roleHits == 0 && strings.TrimSpace(roleBasis) == "" {
		roleFit = 8
	}

	score := skillFit + roleFit
	// Provider quality is a small tie-breaker; relevance remains dominant.
	switch strings.ToLower(job.Provider) {
	case "greenhouse":
		score += 2
	case "lever", "ashby":
		score += 1
	case "himalayas", "jobicy":
		score -= 2
	}
	// Treat India as a preference, not a hard filter: include remote listings and
	// reduce explicit non-India matches while preserving other strong-fit roles.
	if prefersIndia(user.Location) {
		location := strings.ToLower(job.Location)
		switch {
		case strings.Contains(location, "india"):
			score += 8
		case containsAny(location, "bengaluru", "bangalore", "hyderabad", "pune", "mumbai", "chennai", "delhi", "noida", "gurugram", "gurgaon", "kolkata", "ahmedabad", "kochi", "coimbatore", "jaipur", "indore"):
			score += 7
		case location == "":
			// Missing location is unknown, so it receives no preference bonus.
		case strings.Contains(location, "remote") || strings.Contains(location, "anywhere"):
			score += 3
		default:
			score -= 5
		}
	}

	if score > 100 {
		score = 100
	}
	reason := "Resume fit from listed skill coverage and target role overlap."
	if len(matchedSkills) > 0 {
		reason = "Matched skills: " + strings.Join(matchedSkills, ", ") + "."
	}
	if score < 25 {
		reason = "Low resume/profile overlap. " + reason
	}

	return MatchResult{Score: score, Reason: reason, MatchedSkills: matchedSkills}
}

func catalogSkillsInText(text string) []string {
	var found []string
	lower := normalizedPhraseText(text)
	for _, skill := range skillCatalog {
		if strings.Contains(lower, normalizedPhraseText(skill)) {
			found = append(found, skill)
		}
	}
	return found
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
