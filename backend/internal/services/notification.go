package services

import (
	"fmt"
	"log"
)

// SendNotification sends a notification (mocked for now) via Email and WhatsApp
func SendNotification(userID uint, email, whatsapp, jobTitle string) {
	log.Printf("================== NOTIFICATION ==================")
	if email != "" {
		fmt.Printf("[EMAIL] To: %s | Subject: New Job Match: %s\n", email, jobTitle)
		fmt.Printf("Message: We found a new job match and generated an application draft for you! Please check your Job Hunter dashboard to review, apply or reject.\n")
	}

	if whatsapp != "" {
		fmt.Printf("[WHATSAPP] To: %s | New Job Draft for: %s\n", whatsapp, jobTitle)
		fmt.Printf("Message: Login to Job Hunter to review your AI-generated draft!\n")
	}
	log.Printf("==================================================")
}
