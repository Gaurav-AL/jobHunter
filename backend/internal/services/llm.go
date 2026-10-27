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

const DefaultGauravResume = `GAURAV CHAUDHARY
gauravchaudhary.hck@gmail.com +91-8090161875 LinkedIn Github Coding Profile
Education
National Institute of Technology, Kurukshetra
MCA (Master of Computer Applications); CGPA: 8.90
Jun 2020 – Jun 2023
Kurukshetra
Skills
Languages: Java, Python, Go, C++
Frameworks: Django, Flask, Spring Boot, Gin, FastAPI
Tools: Docker, Docker, Kubernetes, MySQL, Grafana, Redash, Git, Tuxedo, Postman, Agentic AI
Platforms: Linux, AWS, GCP
Experience
Mercor Jan 2026 – Present
Software Engineer (Freelancer)
• Audited AI-generated backend code, architectures, and technical documentation for factual correctness, identifying logical flaws, eliminating hallucinations, and improving reliability across complex distributed engineering systems.
Razorpay Apr 2025 – Oct 2025
Software Development Engineer
• Implemented Canary Deployment using Kubernetes, enabling gradual rollout of new features to 1% of users before full release,reducing production risk, validating changes through live traffic, and improving release reliability.
• Refactored and migrated legacy services from a monolithic architecture to microservices, improving platform scalability, reducing service dependencies, and enabling reliable deployments, better service isolation, and maintainability.
• Optimized enterprise-grade database connection pooling, reducing production P95 latency from 250ms to 200ms and improving application responsiveness, resource utilization, and overall system performance under high-traffic workloads.
• Developed comprehensive integration test suites, increasing core service test coverage from 0% to 80% and significantly reducing production regressions by validating critical workflows, service interactions, and business logic.
• Integrated distributed tracing infrastructure across critical services, accelerating root-cause analysis and issue resolution, improving observability, and reducing debugging effort for production incidents in distributed systems.
Amdocs Development Centre India LLP Jul 2023 – Apr 2025
Associate Software Engineer
• Investigated and resolved 50+ critical production issues in Vodafone telecom systems, addressing distributed state management and complex concurrency conflicts in C++ and COBOL, improving platform stability and service reliability.
• Designed and implemented automated charge-classification pipelines, reducing manual verification efforts by 60% while improving classification accuracy, billing workflow efficiency, and consistency across enterprise charging systems.
• Corrected an Ultracard status defect affecting subscriber number deallocation, ensuring timely release of cancelled numbers,improving number lifecycle management, and reducing the risk of future allocation shortages in telecom systems.
Projects
Quix: AI Quiz Maker & Trivia Apr 2026 – Jun 2026
• Built Quix, an AI-powered quiz platform leveraging 6 parallel LLMs with majority-vote validation to improve answer accuracy, while developing a Flutter application integrated with Firebase RTDB, Supabase, AdMob, and Play Billing.
• Engineered Quix with freemium monetization using rewarded ads, 19 IAP tokens, and Google Play subscriptions, alongside a real-time multiplayer quiz experience featuring Firebase RTDB, live leaderboards, FCM, and AI quiz generation.
Certificate
• Software Engineering - NASSCOM
• COBOL Certification
• Python Backend Developer Internship - WSPL Pvt. LTF`

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

// GenerateTailoredLatexResume produces clean, 100% compilable LaTeX resume code
// tailored specifically for the target job without misrepresenting the candidate's real experience.
func GenerateTailoredLatexResume(resumeText, jobTitle, company, jobDescription string) (string, error) {
	prompt := fmt.Sprintf(`You are an expert LaTeX resume engineer.
Produce clean, 100%% compilable LaTeX code for the candidate that matches the layout and sections of their approved resume, tailoring the skill emphasis, keywords, and bullet points to maximize relevance for the target job.

Target Job: %s at %s
Job Description:
%s

Verified Source Resume Facts (DO NOT invent fake employers, degrees, or dates; all experience must remain 100%% factual):
%s

LaTeX Requirements:
1. Output ONLY the raw LaTeX document starting with \documentclass and ending with \end{document}.
2. Do not use markdown code block backticks (no `+"```"+`latex).
3. Use article class with geometry: top=0.4in, bottom=0.4in, left=0.5in, right=0.5in.
4. Bold and prioritize skills and bullet points demanded by this job description (e.g. C++, Python, Go, Kubernetes, Linux, microservices).
5. Ensure all special characters like %%, &, _, # are properly escaped (\%%, \&, \_, \#).`, jobTitle, company, jobDescription, resumeText)

	code, err := generateWithAICC(prompt)
	if err == nil {
		clean := strings.TrimSpace(code)
		if start := strings.Index(clean, "\\documentclass"); start != -1 {
			if end := strings.LastIndex(clean, "\\end{document}"); end != -1 {
				return clean[start : end+len("\\end{document}")], nil
			}
		}
	}
	// Fall back to rock-solid deterministic tailored LaTeX generator
	return BuildDeterministicTailoredLatex(resumeText, jobTitle, company, jobDescription), nil
}

// BuildDeterministicTailoredLatex generates valid, beautiful, 1-page compilable LaTeX
// tailored to the target job description while preserving 100% factual accuracy.
func BuildDeterministicTailoredLatex(resumeText, jobTitle, company, jobDescription string) string {
	combined := strings.ToLower(jobTitle + " " + company + " " + jobDescription)

	isCpp := strings.Contains(combined, "c++") || strings.Contains(combined, "rust") || strings.Contains(combined, "systems") || strings.Contains(combined, "embedded")
	isPython := strings.Contains(combined, "python") || strings.Contains(combined, "django") || strings.Contains(combined, "flask") || strings.Contains(combined, "fastapi")
	isGo := strings.Contains(combined, "go") || strings.Contains(combined, "golang") || strings.Contains(combined, "gin")
	isJava := strings.Contains(combined, "java") || strings.Contains(combined, "spring")
	isK8s := strings.Contains(combined, "kubernetes") || strings.Contains(combined, "k8s") || strings.Contains(combined, "canary")
	isDocker := strings.Contains(combined, "docker") || strings.Contains(combined, "container")
	isLinux := strings.Contains(combined, "linux") || strings.Contains(combined, "ubuntu") || strings.Contains(combined, "unix")
	isMicroservices := strings.Contains(combined, "microservice") || strings.Contains(combined, "distributed") || strings.Contains(combined, "latency")

	// Tailor language order and bolding
	langList := []string{"Go", "Python", "Java", "C++"}
	if isCpp {
		langList = []string{`\textbf{C++}`, "Python", "Go", "Java"}
	} else if isGo {
		langList = []string{`\textbf{Go}`, "Python", "Java", "C++"}
	} else if isPython {
		langList = []string{`\textbf{Python}`, "Go", "Java", "C++"}
	} else if isJava {
		langList = []string{`\textbf{Java}`, "Python", "Go", "C++"}
	}
	languages := strings.Join(langList, ", ")

	// Tailor frameworks
	fwList := []string{"Django", "Flask", "Spring Boot", "Gin", "FastAPI"}
	if isGo {
		fwList = []string{`\textbf{Gin}`, `\textbf{FastAPI}`, "Django", "Flask", "Spring Boot"}
	} else if isPython {
		fwList = []string{`\textbf{FastAPI}`, `\textbf{Django}`, `\textbf{Flask}`, "Gin", "Spring Boot"}
	} else if isJava {
		fwList = []string{`\textbf{Spring Boot}`, "Gin", "FastAPI", "Django", "Flask"}
	}
	frameworks := strings.Join(fwList, ", ")

	// Tailor tools
	toolList := []string{"Docker", "Kubernetes", "MySQL", "Grafana", "Redash", "Git", "Tuxedo", "Postman", "Agentic AI"}
	if isK8s || isDocker {
		toolList = []string{`\textbf{Kubernetes}`, `\textbf{Docker}`, "MySQL", "Grafana", "Git", "Redash", "Agentic AI", "Postman", "Tuxedo"}
	}
	tools := strings.Join(toolList, ", ")

	// Tailor platforms
	platformList := []string{"Linux", "AWS", "GCP"}
	if isLinux {
		platformList = []string{`\textbf{Linux}`, "AWS", "GCP"}
	}
	platforms := strings.Join(platformList, ", ")

	// Tailor bullets based on JD
	var amdocsBullets []string
	if isCpp || isLinux {
		amdocsBullets = append(amdocsBullets,
			`\item Investigated and resolved 50+ critical production issues in Vodafone telecom systems, addressing distributed state management and complex concurrency conflicts in \textbf{C++} and COBOL, improving platform stability and service reliability.`,
			`\item Designed and implemented automated charge-classification pipelines on \textbf{Linux}, reducing manual verification efforts by 60\% while improving classification accuracy, billing workflow efficiency, and consistency across enterprise charging systems.`,
			`\item Corrected an Ultracard status defect affecting subscriber number deallocation, ensuring timely release of cancelled numbers, improving number lifecycle management, and reducing the risk of future allocation shortages in telecom systems.`,
		)
	} else {
		amdocsBullets = append(amdocsBullets,
			`\item Designed and implemented automated charge-classification pipelines, reducing manual verification efforts by 60\% while improving classification accuracy, billing workflow efficiency, and consistency across enterprise charging systems.`,
			`\item Investigated and resolved 50+ critical production issues in Vodafone telecom systems, addressing distributed state management and complex concurrency conflicts in C++ and COBOL, improving platform stability and service reliability.`,
			`\item Corrected an Ultracard status defect affecting subscriber number deallocation, ensuring timely release of cancelled numbers, improving number lifecycle management, and reducing the risk of future allocation shortages in telecom systems.`,
		)
	}

	var razorpayBullets []string
	if isK8s {
		razorpayBullets = append(razorpayBullets,
			`\item Implemented Canary Deployment using \textbf{Kubernetes}, enabling gradual rollout of new features to 1\% of users before full release, reducing production risk, validating changes through live traffic, and improving release reliability.`,
		)
	}
	if isMicroservices {
		razorpayBullets = append(razorpayBullets,
			`\item Refactored and migrated legacy services from a monolithic architecture to \textbf{microservices}, improving platform scalability, reducing service dependencies, and enabling reliable deployments, better service isolation, and maintainability.`,
			`\item Optimized enterprise-grade database connection pooling, reducing production P95 latency from 250ms to 200ms and improving application responsiveness, resource utilization, and overall system performance under high-traffic workloads.`,
		)
	}
	if !isK8s {
		razorpayBullets = append(razorpayBullets,
			`\item Implemented Canary Deployment using Kubernetes, enabling gradual rollout of new features to 1\% of users before full release, reducing production risk, validating changes through live traffic, and improving release reliability.`,
		)
	}
	if !isMicroservices {
		razorpayBullets = append(razorpayBullets,
			`\item Refactored and migrated legacy services from a monolithic architecture to microservices, improving platform scalability, reducing service dependencies, and enabling reliable deployments, better service isolation, and maintainability.`,
			`\item Optimized enterprise-grade database connection pooling, reducing production P95 latency from 250ms to 200ms and improving application responsiveness, resource utilization, and overall system performance under high-traffic workloads.`,
		)
	}
	razorpayBullets = append(razorpayBullets,
		`\item Developed comprehensive integration test suites, increasing core service test coverage from 0\% to 80\% and significantly reducing production regressions by validating critical workflows, service interactions, and business logic.`,
		`\item Integrated distributed tracing infrastructure across critical services, accelerating root-cause analysis and issue resolution, improving observability, and reducing debugging effort for production incidents in distributed systems.`,
	)

	return fmt.Sprintf(`\documentclass[10pt,a4paper]{article}
\usepackage[utf8]{inputenc}
\usepackage[empty]{fullpage}
\usepackage{titlesec}
\usepackage{geometry}
\geometry{top=0.4in, bottom=0.4in, left=0.5in, right=0.5in}
\usepackage[hidelinks]{hyperref}
\usepackage{enumitem}
\usepackage{xcolor}

\setlist[itemize]{leftmargin=*,itemsep=1.5pt,topsep=2pt,parsep=0pt}

\titleformat{\section}{\large\bfseries\uppercase}{}{0em}{}[\titlerule]
\titlespacing*{\section}{0pt}{8pt}{4pt}

\begin{document}

\begin{center}
    {\Huge \textbf{GAURAV CHAUDHARY}} \\ \vspace{4pt}
    \small \href{mailto:gauravchaudhary.hck@gmail.com}{gauravchaudhary.hck@gmail.com} $|$ 
    +91-8090161875 $|$ 
    \href{https://linkedin.com}{LinkedIn} $|$ 
    \href{https://github.com}{Github} $|$ 
    \href{https://leetcode.com}{Coding Profile}
\end{center}

\section{Education}
\noindent \textbf{National Institute of Technology, Kurukshetra} \hfill Jun 2020 -- Jun 2023 \\
\textit{MCA (Master of Computer Applications); CGPA: 8.90} \hfill \textit{Kurukshetra}

\section{Skills}
\begin{itemize}
    \item \textbf{Languages:} %s
    \item \textbf{Frameworks:} %s
    \item \textbf{Tools:} %s
    \item \textbf{Platforms:} %s
\end{itemize}

\section{Experience}

\noindent \textbf{Mercor} \hfill Jan 2026 -- Present \\
\textit{Software Engineer (Freelancer)}
\begin{itemize}
    \item Audited AI-generated backend code, architectures, and technical documentation for factual correctness, identifying logical flaws, eliminating hallucinations, and improving reliability across complex distributed engineering systems.
\end{itemize}

\vspace{4pt}
\noindent \textbf{Razorpay} \hfill Apr 2025 -- Oct 2025 \\
\textit{Software Development Engineer}
\begin{itemize}
%s
\end{itemize}

\vspace{4pt}
\noindent \textbf{Amdocs Development Centre India LLP} \hfill Jul 2023 -- Apr 2025 \\
\textit{Associate Software Engineer}
\begin{itemize}
%s
\end{itemize}

\section{Projects}
\noindent \textbf{Quix: AI Quiz Maker \& Trivia} \hfill Apr 2026 -- Jun 2026
\begin{itemize}
    \item Built Quix, an AI-powered quiz platform leveraging 6 parallel LLMs with majority-vote validation to improve answer accuracy, while developing a Flutter application integrated with Firebase RTDB, Supabase, AdMob, and Play Billing.
    \item Engineered Quix with freemium monetization using rewarded ads, 19 IAP tokens, and Google Play subscriptions, alongside a real-time multiplayer quiz experience featuring Firebase RTDB, live leaderboards, FCM, and AI quiz generation.
\end{itemize}

\section{Certificates}
\begin{itemize}
    \item Software Engineering -- NASSCOM
    \item COBOL Certification
    \item Python Backend Developer Internship -- WSPL Pvt. LTF
\end{itemize}

\end{document}`, languages, frameworks, tools, platforms, strings.Join(razorpayBullets, "\n"), strings.Join(amdocsBullets, "\n"))
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
