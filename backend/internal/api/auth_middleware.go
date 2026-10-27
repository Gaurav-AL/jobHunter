package api

import (
	"crypto"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const firebaseCertURL = "https://www.googleapis.com/service_accounts/v1/metadata/x509/securetoken@system.gserviceaccount.com"

var (
	certMu      sync.RWMutex
	certCache   map[string]*rsa.PublicKey
	certExpires time.Time
)

type SessionClaims struct {
	UserID      uint   `json:"user_id"`
	Email       string `json:"email"`
	FirebaseUID string `json:"firebase_uid"`
	IssuedAt    int64  `json:"iat"`
	ExpiresAt   int64  `json:"exp"`
}

func getSessionSecret() []byte {
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secret == "" {
		secret = "jobhunter-2hr-session-secret-key-stable-dev"
	}
	return []byte(secret)
}

// GenerateSessionToken creates an HMAC-SHA256 signed JWT valid for strictly 2 hours.
func GenerateSessionToken(userID uint, email, firebaseUID string) (string, int64, error) {
	now := time.Now()
	expiresAt := now.Add(2 * time.Hour).Unix()

	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", 0, err
	}

	claims := SessionClaims{
		UserID:      userID,
		Email:       email,
		FirebaseUID: firebaseUID,
		IssuedAt:    now.Unix(),
		ExpiresAt:   expiresAt,
	}
	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		return "", 0, err
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString(headerBytes)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsBytes)
	message := encodedHeader + "." + encodedClaims

	mac := hmac.New(sha256.New, getSessionSecret())
	mac.Write([]byte(message))
	signature := mac.Sum(nil)
	encodedSignature := base64.RawURLEncoding.EncodeToString(signature)

	return message + "." + encodedSignature, expiresAt, nil
}

// VerifySessionToken validates the 2-hour session JWT token.
func VerifySessionToken(tokenString string) (*SessionClaims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed session token")
	}

	message := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, getSessionSecret())
	mac.Write([]byte(message))
	expectedSignature := mac.Sum(nil)

	signatureBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding")
	}

	if !hmac.Equal(signatureBytes, expectedSignature) {
		return nil, fmt.Errorf("invalid session signature")
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid claims encoding")
	}

	var claims SessionClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, fmt.Errorf("invalid claims json")
	}

	now := time.Now().Unix()
	if claims.ExpiresAt <= now {
		return nil, fmt.Errorf("session token expired")
	}

	return &claims, nil
}

type firebaseTokenHeader struct {
	Alg   string `json:"alg"`
	KeyID string `json:"kid"`
}
type firebaseTokenClaims struct {
	Issuer    string `json:"iss"`
	Audience  any    `json:"aud"`
	Subject   string `json:"sub"`
	Email     string `json:"email"`
	ExpiresAt int64  `json:"exp"`
	IssuedAt  int64  `json:"iat"`
	AuthTime  int64  `json:"auth_time"`
	Firebase  struct {
		SignInProvider string `json:"sign_in_provider"`
	} `json:"firebase"`
}

const temporaryEmailLinkSessionLifetime = 2 * time.Hour

func FirebaseAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/api/integrations/health" || strings.HasPrefix(c.Request.URL.Path, "/api/internal/") || strings.HasPrefix(c.Request.URL.Path, "/api/public/") {
			c.Next()
			return
		}

		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization Bearer token is required"})
			return
		}
		rawToken := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))

		// 1. First, check if the request provides a valid 2-Hour Session JWT.
		// If valid, session verification completes purely in-memory (0.05ms) with zero DB/Google lookups.
		if sessionClaims, err := VerifySessionToken(rawToken); err == nil {
			c.Set("user_id", sessionClaims.UserID)
			c.Set("firebase_uid", sessionClaims.FirebaseUID)
			c.Set("firebase_email", sessionClaims.Email)
			c.Next()
			return
		}

		// 2. Otherwise verify as a direct Firebase ID token (used during initial login / session exchange)
		projectID := strings.TrimSpace(os.Getenv("FIREBASE_PROJECT_ID"))
		if projectID == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "Firebase auth is not configured on the server"})
			return
		}
		uid, email, err := verifyFirebaseIDToken(rawToken, projectID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Session token is invalid or expired. Please sign in again."})
			return
		}
		c.Set("firebase_uid", uid)
		c.Set("firebase_email", email)
		c.Next()
	}
}

func verifyFirebaseIDToken(rawToken, projectID string) (string, string, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return "", "", fmt.Errorf("malformed token")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", err
	}
	var header firebaseTokenHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil || header.Alg != "RS256" || header.KeyID == "" {
		return "", "", fmt.Errorf("unsupported token header")
	}
	keys, err := firebaseKeys()
	if err != nil {
		return "", "", err
	}
	key := keys[header.KeyID]
	if key == nil {
		return "", "", fmt.Errorf("unknown token signing key")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return "", "", fmt.Errorf("invalid token signature")
	}
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", err
	}
	var claims firebaseTokenClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return "", "", err
	}
	now := time.Now().Unix()
	if claims.Issuer != "https://securetoken.google.com/"+projectID || !audienceMatches(claims.Audience, projectID) || claims.Subject == "" || len(claims.Subject) > 128 {
		return "", "", fmt.Errorf("invalid token identity")
	}
	if claims.ExpiresAt == 0 || now >= claims.ExpiresAt || claims.IssuedAt == 0 || claims.IssuedAt > now+30 || claims.AuthTime == 0 || claims.AuthTime > now+30 {
		return "", "", fmt.Errorf("invalid token lifetime")
	}
	if claims.Firebase.SignInProvider == "emailLink" && now-claims.AuthTime > int64(temporaryEmailLinkSessionLifetime.Seconds()) {
		return "", "", fmt.Errorf("temporary email-link session expired")
	}
	return claims.Subject, claims.Email, nil
}

func audienceMatches(audience any, projectID string) bool {
	switch value := audience.(type) {
	case string:
		return value == projectID
	case []any:
		for _, item := range value {
			if item == projectID {
				return true
			}
		}
	}
	return false
}

func firebaseKeys() (map[string]*rsa.PublicKey, error) {
	certMu.RLock()
	if time.Now().Before(certExpires) && len(certCache) > 0 {
		keys := certCache
		certMu.RUnlock()
		return keys, nil
	}
	certMu.RUnlock()
	certMu.Lock()
	defer certMu.Unlock()
	if time.Now().Before(certExpires) && len(certCache) > 0 {
		return certCache, nil
	}
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(firebaseCertURL)
	if err != nil {
		return nil, fmt.Errorf("fetch Firebase signing keys: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Firebase signing keys returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var certificates map[string]string
	if err := json.Unmarshal(body, &certificates); err != nil {
		return nil, err
	}
	keys := make(map[string]*rsa.PublicKey, len(certificates))
	for kid, certificatePEM := range certificates {
		block, _ := pem.Decode([]byte(certificatePEM))
		if block == nil {
			continue
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		key, ok := certificate.PublicKey.(*rsa.PublicKey)
		if ok {
			keys[kid] = key
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("no Firebase RSA signing keys returned")
	}
	cacheSeconds := 300
	if match := regexp.MustCompile(`max-age=(\d+)`).FindStringSubmatch(response.Header.Get("Cache-Control")); len(match) == 2 {
		if parsed, err := strconv.Atoi(match[1]); err == nil && parsed > 0 && parsed < 86400 {
			cacheSeconds = parsed
		}
	}
	certCache, certExpires = keys, time.Now().Add(time.Duration(cacheSeconds)*time.Second)
	return keys, nil
}
