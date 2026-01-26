package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
)

const (
	stsService          = "sts"
	snowflakeAudience   = "snowflakecomputing.com"
	getCallerIdentityV  = "2011-06-15"
)

// wifAttestation represents the attestation object for Snowflake WIF authentication.
type wifAttestation struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
}

// getAWSWIFToken generates a WIF attestation token for AWS authentication.
// It creates a signed STS GetCallerIdentity request and packages it as a base64-encoded JSON.
func getAWSWIFToken(ctx context.Context) (string, error) {
	// Load AWS configuration from environment
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("load AWS config: %w", err)
	}

	// Get AWS credentials
	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return "", fmt.Errorf("retrieve AWS credentials: %w", err)
	}

	// Determine the region
	region := cfg.Region
	if region == "" {
		region = "us-east-1" // Default region for STS global endpoint
	}

	// Create the attestation
	attestation, err := createAWSAttestation(ctx, creds, region)
	if err != nil {
		return "", err
	}

	// Encode attestation as JSON and then base64
	attestationJSON, err := json.Marshal(attestation)
	if err != nil {
		return "", fmt.Errorf("marshal attestation: %w", err)
	}

	return base64.StdEncoding.EncodeToString(attestationJSON), nil
}

// createAWSAttestation creates a signed STS GetCallerIdentity request attestation.
func createAWSAttestation(ctx context.Context, creds aws.Credentials, region string) (*wifAttestation, error) {
	// Build STS GetCallerIdentity URL
	stsURL := fmt.Sprintf("https://sts.%s.amazonaws.com/?Action=GetCallerIdentity&Version=%s", region, getCallerIdentityV)

	// Create the request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, stsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create STS request: %w", err)
	}

	// Set required headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Snowflake-Audience", snowflakeAudience)

	// Calculate payload hash (empty body)
	payloadHash := sha256.Sum256([]byte{})
	payloadHashHex := hex.EncodeToString(payloadHash[:])

	// Sign the request using AWS Signature Version 4
	signer := v4.NewSigner()
	signTime := time.Now().UTC()

	err = signer.SignHTTP(ctx, creds, req, payloadHashHex, stsService, region, signTime)
	if err != nil {
		return nil, fmt.Errorf("sign STS request: %w", err)
	}

	// Extract signed headers for attestation
	headers := make(map[string]string)
	for key := range req.Header {
		// Include all headers that were set
		headers[key] = req.Header.Get(key)
	}

	// Also include Host header
	headers["Host"] = req.Host

	return &wifAttestation{
		URL:     stsURL,
		Method:  http.MethodPost,
		Headers: headers,
	}, nil
}

// sortedHeaderKeys returns header keys in sorted order for consistent output.
func sortedHeaderKeys(headers map[string]string) []string {
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// IsAWSEnvironment checks if AWS credentials are available in the environment.
func IsAWSEnvironment(ctx context.Context) bool {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return false
	}
	_, err = cfg.Credentials.Retrieve(ctx)
	return err == nil
}

// isAWSEnvironment is an internal alias for IsAWSEnvironment.
func isAWSEnvironment(ctx context.Context) bool {
	return IsAWSEnvironment(ctx)
}

// Ensure the sortedHeaderKeys function is available (may be used for debugging)
var _ = sortedHeaderKeys

// normalizeAWSRegion ensures a valid AWS region is used.
func normalizeAWSRegion(region string) string {
	region = strings.TrimSpace(region)
	if region == "" {
		return "us-east-1"
	}
	return region
}
