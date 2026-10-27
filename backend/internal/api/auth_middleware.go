package api

import (
	"crypto"
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

type firebaseTokenHeader struct {
	Alg   string `json:"alg"`
	KeyID string `json:"kid"`
}
type firebaseTokenClaims struct {
	Issuer    string `json:"iss"`
	Audience  any    `json:"aud"`
	Subject   string `json:"sub"`
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
		projectID := strings.TrimSpace(os.Getenv("FIREBASE_PROJECT_ID"))
		if projectID == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "Firebase auth is not configured on the server"})
			return
		}
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Firebase ID token is required"})
			return
		}
		uid, err := verifyFirebaseIDToken(strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")), projectID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Firebase ID token is invalid or expired"})
			return
		}
		c.Set("firebase_uid", uid)
		c.Next()
	}
}

func verifyFirebaseIDToken(rawToken, projectID string) (string, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("malformed token")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", err
	}
	var header firebaseTokenHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil || header.Alg != "RS256" || header.KeyID == "" {
		return "", fmt.Errorf("unsupported token header")
	}
	keys, err := firebaseKeys()
	if err != nil {
		return "", err
	}
	key := keys[header.KeyID]
	if key == nil {
		return "", fmt.Errorf("unknown token signing key")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return "", fmt.Errorf("invalid token signature")
	}
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	var claims firebaseTokenClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return "", err
	}
	now := time.Now().Unix()
	if claims.Issuer != "https://securetoken.google.com/"+projectID || !audienceMatches(claims.Audience, projectID) || claims.Subject == "" || len(claims.Subject) > 128 {
		return "", fmt.Errorf("invalid token identity")
	}
	if claims.ExpiresAt == 0 || now >= claims.ExpiresAt || claims.IssuedAt == 0 || claims.IssuedAt > now+30 || claims.AuthTime == 0 || claims.AuthTime > now+30 {
		return "", fmt.Errorf("invalid token lifetime")
	}
	if claims.Firebase.SignInProvider == "emailLink" && now-claims.AuthTime > int64(temporaryEmailLinkSessionLifetime.Seconds()) {
		return "", fmt.Errorf("temporary email-link session expired")
	}
	return claims.Subject, nil
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
