package services

import (
	"os"
	"strings"
	"testing"
)

func TestGenerateDraftRequiresAICCKey(t *testing.T) {
	originalKey := os.Getenv("AICC_KEY")
	t.Setenv("AICC_KEY", "")
	t.Cleanup(func() {
		if originalKey != "" {
			t.Setenv("AICC_KEY", originalKey)
		}
	})

	_, err := GenerateDraft("resume", "job")
	if err == nil || !strings.Contains(err.Error(), "AICC_KEY") {
		t.Fatalf("expected missing AICC_KEY error, got %v", err)
	}
}

func TestGenerateDraftLive(t *testing.T) {
	if os.Getenv("RUN_LIVE_LLM_CHECK") != "1" {
		t.Skip("set RUN_LIVE_LLM_CHECK=1 to verify the configured LLM key against the provider")
	}
	if os.Getenv("AICC_KEY") == "" {
		t.Skip("AICC_KEY is not configured")
	}

	draft, err := GenerateDraft(
		"Go and Vue developer with backend API experience.",
		"Build and maintain Go APIs for a Vue application.",
	)
	if err != nil {
		t.Fatalf("expected live LLM generation to succeed: %v", err)
	}
	if strings.TrimSpace(draft) == "" {
		t.Fatal("expected live LLM generation to return content")
	}
}
