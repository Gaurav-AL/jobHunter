package services

import (
	"context"
	"fmt"
	"os"
	"strings"

	"backend/internal/repository"
	"google.golang.org/genai"
)

func generateWithAICC(prompt string) (string, error) {
	apiKey := os.Getenv("AICC_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("AICC_KEY environment variable is not set")
	}

	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return "", err
	}

	resp, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", genai.Text(prompt), nil)
	if err != nil {
		return "", err
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content generated")
	}

	return strings.TrimSpace(resp.Candidates[0].Content.Parts[0].Text), nil
}

// GenerateDraft uses AICC_KEY to generate a personalized job application draft.
func GenerateDraft(resumeText, jobDescription string) (string, error) {
	prompt := fmt.Sprintf(`You are an expert career coach. Based on the provided resume/profile and job description, draft a highly compelling and personalized cover letter or application message.
	
Resume/Profile:
%s

Job Description:
%s

Draft an application that highlights the best matching skills. Keep it professional, concise, and ready to send.`, resumeText, jobDescription)

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

// GenerateTailoredResume produces a role-focused, ATS-readable resume draft.
// The prompt forbids inventing qualifications, dates, employers, and metrics.
func GenerateTailoredResume(resumeText, jobTitle, jobDescription string) (string, error) {
	prompt := fmt.Sprintf(`You are editing a candidate's resume for a specific job. Return only an ATS-friendly plain-text resume draft. Use only facts explicitly present in the source resume. Do not invent skills, employers, dates, degrees, accomplishments, metrics, or certifications. You may reorder existing skills and bullets, shorten wording without changing meaning, and emphasize relevant evidence. Preserve names, contact details, job titles, employers, dates, and factual claims exactly. If the source resume does not support a job requirement, do not claim it.

Target job title:
%s

Target job description:
%s

Source resume and profile:
%s
`, jobTitle, jobDescription, resumeText)
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
