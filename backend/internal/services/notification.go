package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 12 * time.Second}

type twilioResult struct {
	SID     string `json:"sid"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func TwilioVerifyConfigured() bool {
	return os.Getenv("TWILIO_ACCOUNT_SID") != "" && os.Getenv("TWILIO_AUTH_TOKEN") != "" && os.Getenv("TWILIO_VERIFY_SERVICE_SID") != ""
}

func TwilioSMSConfigured() bool {
	return os.Getenv("TWILIO_ACCOUNT_SID") != "" && os.Getenv("TWILIO_AUTH_TOKEN") != "" && (os.Getenv("TWILIO_MESSAGING_SERVICE_SID") != "" || os.Getenv("TWILIO_FROM_NUMBER") != "")
}

func RequestPhoneVerification(phone string) error {
	if !TwilioVerifyConfigured() {
		return fmt.Errorf("SMS verification is not configured: set Twilio Verify credentials")
	}
	form := url.Values{"To": {phone}, "Channel": {"sms"}}
	var result twilioResult
	path := "https://verify.twilio.com/v2/Services/" + url.PathEscape(os.Getenv("TWILIO_VERIFY_SERVICE_SID")) + "/Verifications"
	return postTwilio(path, form, &result)
}

func CheckPhoneVerification(phone, code string) (bool, error) {
	if !TwilioVerifyConfigured() {
		return false, fmt.Errorf("SMS verification is not configured")
	}
	form := url.Values{"To": {phone}, "Code": {strings.TrimSpace(code)}}
	var result twilioResult
	path := "https://verify.twilio.com/v2/Services/" + url.PathEscape(os.Getenv("TWILIO_VERIFY_SERVICE_SID")) + "/VerificationCheck"
	if err := postTwilio(path, form, &result); err != nil {
		return false, err
	}
	return result.Status == "approved", nil
}

func SendJobSMS(phone, company, title, jobURL string) (string, string, error) {
	if !TwilioSMSConfigured() {
		return "not_configured", "", fmt.Errorf("SMS provider is not configured")
	}
	message := fmt.Sprintf("JobHunter match: %s — %s. Open the employer listing to apply: %s", company, title, jobURL)
	form := url.Values{"To": {phone}, "Body": {message}}
	if serviceSID := os.Getenv("TWILIO_MESSAGING_SERVICE_SID"); serviceSID != "" {
		form.Set("MessagingServiceSid", serviceSID)
	} else {
		form.Set("From", os.Getenv("TWILIO_FROM_NUMBER"))
	}
	var result twilioResult
	path := "https://api.twilio.com/2010-04-01/Accounts/" + url.PathEscape(os.Getenv("TWILIO_ACCOUNT_SID")) + "/Messages.json"
	if err := postTwilio(path, form, &result); err != nil {
		return "failed", "", err
	}
	return result.Status, result.SID, nil
}

func postTwilio(endpoint string, form url.Values, result *twilioResult) error {
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	request.SetBasicAuth(os.Getenv("TWILIO_ACCOUNT_SID"), os.Getenv("TWILIO_AUTH_TOKEN"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("Twilio request failed: %w", err)
	}
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(result); err != nil {
		return fmt.Errorf("Twilio returned HTTP %d", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Twilio rejected request (%d): %s", result.Code, result.Message)
	}
	return nil
}
