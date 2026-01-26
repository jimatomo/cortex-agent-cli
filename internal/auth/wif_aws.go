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
	stsService        = "sts"
	snowflakeAudience = "snowflakecomputing.com"
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
// Uses GET request with query parameters as expected by Snowflake WIF.
// Note: region parameter is kept for potential future use but global STS endpoint is used.
func createAWSAttestation(ctx context.Context, creds aws.Credentials, _ string) (*wifAttestation, error) {
	// Use global STS endpoint with query parameters for GET request
	stsHost := "sts.amazonaws.com"
	stsURL := fmt.Sprintf("https://%s/?Action=GetCallerIdentity&Version=2011-06-15", stsHost)

	// Create a GET request (no body)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create STS request: %w", err)
	}

	// Set Host explicitly
	req.Host = stsHost

	// Set Snowflake audience header
	req.Header.Set("X-Snowflake-Audience", snowflakeAudience)

	// Calculate payload hash (empty body for GET request)
	payloadHash := sha256.Sum256([]byte{})
	payloadHashHex := hex.EncodeToString(payloadHash[:])

	// Sign the request using AWS Signature Version 4
	// Use us-east-1 for global STS endpoint
	signer := v4.NewSigner()
	signTime := time.Now().UTC()

	err = signer.SignHTTP(ctx, creds, req, payloadHashHex, stsService, "us-east-1", signTime)
	if err != nil {
		return nil, fmt.Errorf("sign STS request: %w", err)
	}

	// Extract signed headers for attestation
	headers := make(map[string]string)
	for key := range req.Header {
		headers[key] = req.Header.Get(key)
	}

	// Ensure Host header is included
	headers["Host"] = stsHost

	return &wifAttestation{
		URL:     stsURL,
		Method:  http.MethodGet,
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
