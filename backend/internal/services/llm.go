package services

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/genai"
)

// GenerateDraft uses the AICC_KEY (mapped to Gemini API key) to generate a personalized job application draft.
func GenerateDraft(resumeText, jobDescription string) (string, error) {
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

	prompt := fmt.Sprintf(`You are an expert career coach. Based on the provided resume and job description, draft a highly compelling and personalized cover letter or application message.
	
Resume:
%s

Job Description:
%s

Draft an application that highlights the best matching skills. Keep it professional, concise, and ready to send.`, resumeText, jobDescription)

	resp, err := client.Models.GenerateContent(ctx, "gemini-2.5-flash", genai.Text(prompt), nil)
	if err != nil {
		return "", err
	}
	
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no content generated")
	}
	
	return resp.Candidates[0].Content.Parts[0].Text, nil
}
