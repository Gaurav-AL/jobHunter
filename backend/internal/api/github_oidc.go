package api

import (
    "crypto"
    "crypto/rsa"
    "crypto/sha256"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "io"
    "math/big"
    "net/http"
    "strings"
    "sync"
    "time"

    "github.com/gin-gonic/gin"
)

const (
    githubOIDCIssuer = "https://token.actions.githubusercontent.com"
    githubOIDCAudience = "https://jobhunter-1d1b1.web.app/internal/scheduler/run"
    githubOIDCRepository = "Gaurav-AL/jobHunter"
    githubOIDCWorkflow = ".github/workflows/daily-job-digest.yml@refs/heads/main"
    githubOIDCJWKSURL = "https://token.actions.githubusercontent.com/.well-known/jwks"
)

type githubJWKSet struct { Keys []struct { KID string `json:"kid"`; KTY string `json:"kty"`; N string `json:"n"`; E string `json:"e"` } `json:"keys"` }
type githubJWTClaims struct { Issuer string `json:"iss"`; Audience any `json:"aud"`; Repository string `json:"repository"`; WorkflowRef string `json:"workflow_ref"`; Exp int64 `json:"exp"`; Nbf int64 `json:"nbf"` }
type githubJWTHeader struct { Alg string `json:"alg"`; KID string `json:"kid"` }

var githubKeys struct { sync.RWMutex; expires time.Time; keys map[string]*rsa.PublicKey }

func GitHubActionsAuth() gin.HandlerFunc {
    return func(c *gin.Context) {
        header := strings.TrimSpace(c.GetHeader("Authorization"))
        if !strings.HasPrefix(header, "Bearer ") { c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error":"GitHub Actions identity token is required"}); return }
        if err := verifyGitHubActionsToken(strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))); err != nil { c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error":"GitHub Actions identity token is invalid"}); return }
        c.Next()
    }
}

func verifyGitHubActionsToken(token string) error {
    parts := strings.Split(token, ".")
    if len(parts) != 3 { return fmt.Errorf("malformed JWT") }
    headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0]); if err != nil { return err }
    var header githubJWTHeader
    if json.Unmarshal(headerBytes, &header) != nil || header.Alg != "RS256" || header.KID == "" { return fmt.Errorf("invalid JWT header") }
    keys, err := getGitHubOIDCKeys(); if err != nil { return err }
    key := keys[header.KID]; if key == nil { return fmt.Errorf("unknown signing key") }
    signature, err := base64.RawURLEncoding.DecodeString(parts[2]); if err != nil { return err }
    digest := sha256.Sum256([]byte(parts[0]+"."+parts[1]))
    if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil { return err }
    claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1]); if err != nil { return err }
    var claims githubJWTClaims
    if err := json.Unmarshal(claimsBytes, &claims); err != nil { return err }
    now := time.Now().Unix()
    if claims.Issuer != githubOIDCIssuer || claims.Repository != githubOIDCRepository || claims.WorkflowRef != githubOIDCWorkflow || claims.Exp <= now || (claims.Nbf != 0 && claims.Nbf > now+30) || !audienceMatches(claims.Audience, githubOIDCAudience) { return fmt.Errorf("unexpected token claims") }
    return nil
}

func getGitHubOIDCKeys() (map[string]*rsa.PublicKey, error) {
    githubKeys.RLock(); if time.Now().Before(githubKeys.expires) && len(githubKeys.keys)>0 { keys:=githubKeys.keys; githubKeys.RUnlock(); return keys,nil }; githubKeys.RUnlock()
    client:=http.Client{Timeout:5*time.Second}; response,err:=client.Get(githubOIDCJWKSURL); if err!=nil{return nil,err}; defer response.Body.Close(); if response.StatusCode!=http.StatusOK{return nil,fmt.Errorf("GitHub JWKS returned HTTP %d",response.StatusCode)}
    var set githubJWKSet; if err:=json.NewDecoder(io.LimitReader(response.Body,1<<20)).Decode(&set);err!=nil{return nil,err}
    keys:=map[string]*rsa.PublicKey{}
    for _,item:=range set.Keys { if item.KTY!="RSA" || item.KID=="" {continue}; nBytes,e1:=base64.RawURLEncoding.DecodeString(item.N); eBytes,e2:=base64.RawURLEncoding.DecodeString(item.E); if e1!=nil||e2!=nil {continue}; exponent:=0; for _,b:=range eBytes {exponent=(exponent<<8)|int(b)}; if exponent<3 {continue}; keys[item.KID]=&rsa.PublicKey{N:new(big.Int).SetBytes(nBytes),E:exponent} }
    if len(keys)==0{return nil,fmt.Errorf("no GitHub signing keys")}; githubKeys.Lock(); githubKeys.keys=keys; githubKeys.expires=time.Now().Add(15*time.Minute); githubKeys.Unlock(); return keys,nil
}
