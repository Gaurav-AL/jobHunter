package services

import (
	"context"
	"fmt"
	"os"
	"strings"

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
