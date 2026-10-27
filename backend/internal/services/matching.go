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

var nonTechnicalKeywords = []string{
	"account executive", "sales", "business development", "bdr", "sdr",
	"abuse investigator", "fraud investigator", "investigator",
	"recruiter", "talent acquisition", "sourcer", "human resources", "hr ", "people partner",
	"legal", "counsel", "paralegal", "compliance officer",
	"accounting", "bookkeeper", "payroll", "auditor",
	"office manager", "executive assistant", "receptionist",
	"copywriter", "content writer", "seo specialist",
	"customer support", "customer success", "support agent", "support specialist",
	"developer blog", "blog", "community manager", "social media", "event coordinator",
}

var engineeringTitleKeywords = []string{
	"engineer", "developer", "programmer", "sde", "backend", "frontend",
	"fullstack", "full stack", "software", "architect", "devops", "sre",
	"platform", "systems", "infrastructure", "cloud", "data", "ml", "machine learning",
	"distributed", "security engineer", "qa engineer", "tech lead",
}

func MatchJobToProfile(user repository.User, job repository.Job) MatchResult {
	lowerTitle := strings.ToLower(job.Title)

	// Strict filter 1: Instantly disqualify non-engineering roles
	targetRolesLower := strings.ToLower(user.TargetRoles)
	for _, term := range nonTechnicalKeywords {
		if strings.Contains(lowerTitle, term) && !strings.Contains(targetRolesLower, term) {
			return MatchResult{Score: 0, Reason: "Excluded: Non-technical role (" + term + ") incompatible with engineering profile."}
		}
	}

	// Strict filter 2: Role title must exhibit engineering/technical affinity
	isEngineering := false
	for _, keyword := range engineeringTitleKeywords {
		if strings.Contains(lowerTitle, keyword) {
			isEngineering = true
			break
		}
	}
	if !isEngineering && targetRolesLower != "" {
		for _, role := range strings.Split(targetRolesLower, ",") {
			role = strings.TrimSpace(role)
			if role != "" && strings.Contains(lowerTitle, role) {
				isEngineering = true
				break
			}
		}
	}
	if !isEngineering {
		return MatchResult{Score: 0, Reason: "Excluded: Role title is not an engineering or software position."}
	}

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
		skillFit = len(matchedSkills) * 55 / len(jobSkills)
	} else if len(matchedSkills) > 0 {
		skillFit = 30
	} else {
		skillFit = 15
	}

	// Technical role affinity
	roleFit := 15
	if strings.Contains(lowerTitle, "backend") || strings.Contains(lowerTitle, "sde") || strings.Contains(lowerTitle, "software engineer") || strings.Contains(lowerTitle, "systems") || strings.Contains(lowerTitle, "distributed") || strings.Contains(lowerTitle, "golang") || strings.Contains(lowerTitle, "go ") {
		roleFit = 30
	} else if strings.Contains(lowerTitle, "full stack") || strings.Contains(lowerTitle, "fullstack") || strings.Contains(lowerTitle, "platform") {
		roleFit = 25
	}

	// Location alignment
	locFit := 0
	location := strings.ToLower(job.Location + " " + job.Description)
	if prefersIndia(user.Location) {
		switch {
		case strings.Contains(location, "india") || containsAny(location, "bengaluru", "bangalore", "hyderabad", "pune", "mumbai", "chennai", "delhi", "noida", "gurugram", "gurgaon"):
			locFit = 15
		case strings.Contains(location, "remote") || strings.Contains(location, "anywhere") || strings.Contains(location, "apac"):
			locFit = 12
		default:
			// Explicit on-site abroad
			locFit = -10
		}
	}

	score := skillFit + roleFit + locFit
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	reason := "Engineering role match"
	if len(matchedSkills) > 0 {
		reason = "Matched skills: " + strings.Join(matchedSkills, ", ") + "."
	} else {
		reason = "Title alignment with software development profile."
	}
	if score < 50 {
		reason = "Low skill overlap. " + reason
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
