package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"backend/internal/repository"
)

// ApplicantExtraction contains factual resume fields. The prompt asks the model
// to leave unknown values empty; callers merge only into fields the user has not
// already reviewed.
type ApplicantExtraction struct {
	ProfileSummary      string `json:"profile_summary"`
	TargetRoles         string `json:"target_roles"`
	Skills              string `json:"skills"`
	Location            string `json:"location"`
	ApplicantFirstName  string `json:"applicant_first_name"`
	ApplicantLastName   string `json:"applicant_last_name"`
	ApplicantPhone      string `json:"applicant_phone"`
	ApplicantAddress    string `json:"applicant_address"`
	ApplicantCity       string `json:"applicant_city"`
	ApplicantRegion     string `json:"applicant_region"`
	ApplicantPostalCode string `json:"applicant_postal_code"`
	ApplicantCountry    string `json:"applicant_country"`
	ApplicantLinkedIn   string `json:"applicant_linkedin"`
	ApplicantWebsite    string `json:"applicant_website"`
	ApplicantGitHub     string `json:"applicant_github"`
}

func generateWithAICC(prompt string) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("AICC_KEY"))
	if apiKey == "" {
		return "", fmt.Errorf("AICC_KEY environment variable is not set")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("AICC_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = "https://api.ai.cc/v1"
	}
	configuredModel := strings.TrimSpace(os.Getenv("AICC_MODEL"))
	if configuredModel == "" || configuredModel == "gpt-4o-mini" {
		configuredModel = "qwen-plus"
	}

	// Multi-model failover list: try primary first, then robust fallbacks
	candidateModels := []string{configuredModel}
	for _, fallback := range []string{"qwen-plus", "glm-5", "qwen-flash"} {
		if fallback != configuredModel {
			candidateModels = append(candidateModels, fallback)
		}
	}

	client := &http.Client{Timeout: 45 * time.Second}
	var lastErr error

	for _, model := range candidateModels {
		payload, err := json.Marshal(map[string]any{
			"model":       model,
			"messages":    []map[string]string{{"role": "user", "content": prompt}},
			"temperature": 0.2,
		})
		if err != nil {
			return "", fmt.Errorf("encode AICC request: %w", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(payload))
		if err != nil {
			cancel()
			return "", fmt.Errorf("create AICC request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		cancel()
		if err != nil {
			lastErr = fmt.Errorf("AICC request failed for model %s: %w", model, err)
			log.Printf("AICC model %s network failure: %v, trying fallback...", model, err)
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read AICC response for model %s: %w", model, err)
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("AICC returned HTTP %d for model %s: %s", resp.StatusCode, model, strings.TrimSpace(string(body)))
			log.Printf("AICC model %s returned HTTP %d (%s), trying fallback...", model, resp.StatusCode, strings.TrimSpace(string(body)))
			continue
		}

		var completion struct {
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(body, &completion); err != nil {
			lastErr = fmt.Errorf("parse AICC response for model %s: %w", model, err)
			continue
		}
		if len(completion.Choices) == 0 || strings.TrimSpace(completion.Choices[0].Message.Content) == "" {
			lastErr = fmt.Errorf("AICC returned no completion content for model %s", model)
			continue
		}
		log.Printf("AICC usage model=%s prompt_tokens=%d completion_tokens=%d total_tokens=%d", model, completion.Usage.PromptTokens, completion.Usage.CompletionTokens, completion.Usage.TotalTokens)
		return strings.TrimSpace(completion.Choices[0].Message.Content), nil
	}

	return "", fmt.Errorf("all AICC models failed, last error: %w", lastErr)
}

// GenerateDraft uses AICC_KEY to generate a personalized job application draft.
func GenerateDraft(resumeText, jobDescription string) (string, error) {
	prompt := fmt.Sprintf(`You are preparing an editable application note. Use only facts explicitly present in the resume or user-confirmed answers. Never invent skills, employers, metrics, dates, eligibility, notice period, experience, salary, or answers to application questions. If a required detail is missing, omit it and leave it for the candidate to answer. Avoid claiming the candidate has already applied.
	
Resume/Profile and user-confirmed application answers (use notice period, experience, and expected CTC only if the application asks; include a project only if directly relevant or project experience is requested; do not force unrelated information):
%s

Job Description:
%s

Write a concise, professional draft for the candidate to review and edit before submitting on the employer's website.`, resumeText, jobDescription)

	return generateWithAICC(prompt)
}

// GenerateProfileSummary turns extracted resume text into editable profile fields.
func GenerateProfileSummary(resumeText string) (string, error) {
	prompt := fmt.Sprintf(`Extract a concise candidate profile from this resume text.

Return plain text with these headings:
Summary:
Target Roles:
Skills:
Location:

Resume:
%s`, resumeText)

	return generateWithAICC(prompt)
}

// ExtractApplicantProfile sends resume text to the configured AI provider
// and returns only fields supported by the resume. Unknown/legal eligibility
// answers must remain blank for the user to supply.
func ExtractApplicantProfile(resumeText string) (ApplicantExtraction, error) {
	prompt := fmt.Sprintf(`You are an expert HR and recruitment parser. Extract applicant information from this resume.
Return ONLY a valid JSON object (no markdown code blocks, no preamble, no commentary) with exactly these string keys:
- profile_summary: A 2-3 sentence technical summary emphasizing years of experience, core backend/frontend stack, and notable accomplishments.
- target_roles: Comma-separated list of suitable roles derived from the candidate's experience (e.g. "Software Development Engineer, Backend Engineer, Distributed Systems Engineer").
- skills: Comma-separated list of ALL programming languages, frameworks, cloud platforms, databases, and developer tools explicitly found in the resume.
- location: Candidate city and country, or "Remote / Bengaluru, India" if inferred from contact/education.
- applicant_first_name: Candidate's first name.
- applicant_last_name: Candidate's last name.
- applicant_phone: Phone number with country code.
- applicant_address: Street address (if present, else empty string).
- applicant_city: City name (e.g. "Kurukshetra" or "Bengaluru").
- applicant_region: State or region.
- applicant_postal_code: Postal code (if present, else empty string).
- applicant_country: Country (e.g. "India").
- applicant_linkedin: LinkedIn profile URL (if present, else empty string).
- applicant_website: Website or coding profile URL (if present, else empty string).
- applicant_github: GitHub URL (if present, else empty string).

Resume text:
%s`, resumeText)
	response, err := generateWithAICC(prompt)
	if err != nil {
		return ApplicantExtraction{}, err
	}
	response = strings.TrimSpace(response)
	if start := strings.Index(response, "{"); start != -1 {
		if end := strings.LastIndex(response, "}"); end != -1 && end > start {
			response = response[start : end+1]
		}
	}
	var extracted ApplicantExtraction
	if err := json.Unmarshal([]byte(strings.TrimSpace(response)), &extracted); err != nil {
		return ApplicantExtraction{}, fmt.Errorf("could not parse applicant profile response: %w", err)
	}
	return extracted, nil
}

// GenerateTailoredResume produces a role-focused, ATS-readable resume draft.
// The prompt forbids inventing qualifications, dates, employers, and metrics.
func GenerateTailoredResume(resumeText, jobTitle, jobDescription string) (string, error) {
	prompt := fmt.Sprintf(`You are an expert technical career coach and resume writer.
Tailor the candidate's resume for the target job description while strictly adhering to factual accuracy (never invent fake employers, degrees, dates, or certifications).

Target job title:
%s

Target job description:
%s

Candidate source resume:
%s

Formatting & Content Rules:
1. Re-align and emphasize the candidate's verified achievements, metrics, and tech stack that directly match the target job requirements.
2. Structure the tailored resume in clean, professional plain text with uppercase section headers:
   - CANDIDATE CONTACT DETAILS
   - PROFESSIONAL SUMMARY (specifically tailored to emphasize alignment with %s)
   - CORE TECHNICAL SKILLS (Languages, Frameworks, Cloud & DevOps, Databases & Tools - highlighting skills asked in the JD)
   - PROFESSIONAL EXPERIENCE (bullet points tailored to emphasize relevant impact, architecture, and tools)
   - PROJECTS & HIGHLIGHTS
   - EDUCATION & CERTIFICATIONS
3. Maintain ATS-friendly formatting (use standard bullet points •, consistent indentation, and clean text).
4. Output ONLY the tailored resume text without markdown code blocks, explanations, or conversational filler.`, jobTitle, jobDescription, resumeText, jobTitle)
	return generateWithAICC(prompt)
}

// BuildRoleFocusedResume safely creates a deterministic draft when no LLM is configured.
// It preserves the original resume verbatim and only adds factual profile fields and labels.
func BuildRoleFocusedResume(user repository.User, jobTitle, jobDescription string) string {
	var out strings.Builder
	fmt.Fprintf(&out, "RESUME DRAFT — TARGET ROLE: %s\n", strings.TrimSpace(jobTitle))
	out.WriteString("Review this role-focused copy before applying. No new qualifications have been added.\n\n")
	if user.ProfileSummary != "" {
		out.WriteString("PROFILE SUMMARY\n" + strings.TrimSpace(user.ProfileSummary) + "\n\n")
	}
	if user.Skills != "" {
		desc := strings.ToLower(jobDescription)
		var relevant, other []string
		for _, skill := range strings.FieldsFunc(user.Skills, func(r rune) bool { return r == ',' || r == '\n' || r == ';' }) {
			skill = strings.TrimSpace(skill)
			if skill == "" {
				continue
			}
			if strings.Contains(desc, strings.ToLower(skill)) {
				relevant = append(relevant, skill)
			} else {
				other = append(other, skill)
			}
		}
		ordered := append(relevant, other...)
		if len(ordered) > 0 {
			out.WriteString("SKILLS (from your profile)\n" + strings.Join(ordered, ", ") + "\n\n")
		}
	}
	out.WriteString("SOURCE RESUME\n")
	if strings.TrimSpace(user.ResumeText) == "" {
		out.WriteString("Upload a text-based resume to generate a role-focused resume draft.\n")
	} else {
		out.WriteString(strings.TrimSpace(user.ResumeText) + "\n")
	}
	return out.String()
}
