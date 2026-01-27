package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/youmark/pkcs8"
)

const (
	AuthenticatorKeyPair = "KEYPAIR"
)

type Config struct {
	Account              string
	User                 string
	Role                 string
	Warehouse            string
	PrivateKey           string
	PrivateKeyPassphrase string
	Authenticator        string
}

func FromEnv() Config {
	return Config{
		Account:              os.Getenv("SNOWFLAKE_ACCOUNT"),
		User:                 os.Getenv("SNOWFLAKE_USER"),
		Role:                 os.Getenv("SNOWFLAKE_ROLE"),
		Warehouse:            os.Getenv("SNOWFLAKE_WAREHOUSE"),
		PrivateKey:           os.Getenv("SNOWFLAKE_PRIVATE_KEY"),
		PrivateKeyPassphrase: os.Getenv("SNOWFLAKE_PRIVATE_KEY_PASSPHRASE"),
		Authenticator:        envOrDefault("SNOWFLAKE_AUTHENTICATOR", AuthenticatorKeyPair),
	}
}

func AuthHeader(ctx context.Context, cfg Config) (string, error) {
	token, err := BearerToken(ctx, cfg)
	if err != nil {
		return "", err
	}
	return "Bearer " + token, nil
}

func BearerToken(ctx context.Context, cfg Config) (string, error) {
	auth := strings.ToUpper(strings.TrimSpace(cfg.Authenticator))
	if auth == "" {
		auth = AuthenticatorKeyPair
	}

	switch auth {
	case AuthenticatorKeyPair:
		return keyPairJWT(cfg)
	default:
		return "", fmt.Errorf("unsupported authenticator: %s", cfg.Authenticator)
	}
}

func keyPairJWT(cfg Config) (string, error) {
	if cfg.Account == "" || cfg.User == "" || strings.TrimSpace(cfg.PrivateKey) == "" {
		return "", fmt.Errorf("missing required key pair auth settings (SNOWFLAKE_ACCOUNT, SNOWFLAKE_USER, SNOWFLAKE_PRIVATE_KEY)")
	}

	privateKey, publicKey, err := loadKeyPair(cfg.PrivateKey, cfg.PrivateKeyPassphrase)
	if err != nil {
		return "", err
	}

	fingerprint, err := publicKeyFingerprint(publicKey)
	if err != nil {
		return "", err
	}

	account := strings.ToUpper(cfg.Account)
	user := strings.ToUpper(cfg.User)
	now := time.Now().UTC()

	claims := jwt.RegisteredClaims{
		Issuer:    fmt.Sprintf("%s.%s.SHA256:%s", account, user, fingerprint),
		Subject:   fmt.Sprintf("%s.%s", account, user),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(1 * time.Hour)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}
	return signed, nil
}

func loadKeyPair(inline, passphrase string) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	inline = strings.TrimSpace(inline)
	if inline == "" {
		return nil, nil, fmt.Errorf("SNOWFLAKE_PRIVATE_KEY is required")
	}

	raw := strings.ReplaceAll(inline, "\\n", "\n")
	data := []byte(raw)
	privKey, pubKey, err := parsePrivateKey(data, passphrase)
	if err == nil {
		return privKey, pubKey, nil
	}
	// If PEM lines were indented (common in YAML), try de-indenting.
	if normalized := normalizePEM(raw); normalized != raw {
		if privKey, pubKey, normErr := parsePrivateKey([]byte(normalized), passphrase); normErr == nil {
			return privKey, pubKey, nil
		}
	}

	trimmed := strings.TrimSpace(inline)
	if decoded, decodeErr := base64.StdEncoding.DecodeString(trimmed); decodeErr == nil {
		// First try base64-encoded PEM.
		if privKey, pubKey, pemErr := parsePrivateKey(decoded, passphrase); pemErr == nil {
			return privKey, pubKey, nil
		}
		// Also accept base64-encoded DER (PKCS#8 or PKCS#1).
		if derKey, derErr := parsePrivateKeyDER(decoded); derErr == nil {
			return derKey, derKey.Public().(*rsa.PublicKey), nil
		}
	}

	return nil, nil, fmt.Errorf(
		"invalid SNOWFLAKE_PRIVATE_KEY: %w (expected PEM like '-----BEGIN PRIVATE KEY-----' or base64-encoded PEM)",
		err,
	)
}

func normalizePEM(s string) string {
	lines := strings.Split(s, "\n")
	if len(lines) < 2 {
		return s
	}
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent <= 0 {
		return s
	}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) >= minIndent {
			lines[i] = line[minIndent:]
		}
	}
	return strings.Join(lines, "\n")
}

func parsePrivateKey(data []byte, passphrase string) (*rsa.PrivateKey, *rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, nil, fmt.Errorf("invalid PEM")
	}

	var privKey *rsa.PrivateKey
	if x509.IsEncryptedPEMBlock(block) {
		if strings.TrimSpace(passphrase) == "" {
			return nil, nil, fmt.Errorf("private key is encrypted but no passphrase was provided (set SNOWFLAKE_PRIVATE_KEY_PASSPHRASE)")
		}
		der, err := x509.DecryptPEMBlock(block, []byte(passphrase))
		if err != nil {
			return nil, nil, fmt.Errorf("decrypt private key: %w", err)
		}
		key, err := parsePrivateKeyDER(der)
		if err != nil {
			return nil, nil, err
		}
		privKey = key
	} else {
		switch block.Type {
		case "PRIVATE KEY":
			key, parseErr := x509.ParsePKCS8PrivateKey(block.Bytes)
			if parseErr != nil {
				return nil, nil, fmt.Errorf("parse PKCS8 key: %w", parseErr)
			}
			rsaKey, ok := key.(*rsa.PrivateKey)
			if !ok {
				return nil, nil, fmt.Errorf("private key is not RSA")
			}
			privKey = rsaKey
		case "ENCRYPTED PRIVATE KEY":
			if strings.TrimSpace(passphrase) == "" {
				return nil, nil, fmt.Errorf("private key is encrypted but no passphrase was provided (set SNOWFLAKE_PRIVATE_KEY_PASSPHRASE)")
			}
			key, err := pkcs8.ParsePKCS8PrivateKey(block.Bytes, []byte(passphrase))
			if err != nil {
				return nil, nil, fmt.Errorf("decrypt PKCS8 key: %w", err)
			}
			rsaKey, ok := key.(*rsa.PrivateKey)
			if !ok {
				return nil, nil, fmt.Errorf("private key is not RSA")
			}
			privKey = rsaKey
		case "RSA PRIVATE KEY":
			key, parseErr := x509.ParsePKCS1PrivateKey(block.Bytes)
			if parseErr != nil {
				return nil, nil, fmt.Errorf("parse PKCS1 key: %w", parseErr)
			}
			privKey = key
		default:
			return nil, nil, fmt.Errorf("unsupported key type %q", block.Type)
		}
	}

	pubKey := privKey.Public().(*rsa.PublicKey)
	return privKey, pubKey, nil
}

func parsePrivateKeyDER(der []byte) (*rsa.PrivateKey, error) {
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse decrypted key: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA")
	}
	return rsaKey, nil
}

func publicKeyFingerprint(pub *rsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", fmt.Errorf("marshal public key: %w", err)
	}
	sum := sha256.Sum256(der)
	return base64.StdEncoding.EncodeToString(sum[:]), nil
}

func normalizeUser(user string) string {
	trimmed := strings.TrimSpace(user)
	if trimmed == "" {
		return trimmed
	}
	if isSimpleIdentifier(trimmed) {
		return strings.ToUpper(trimmed)
	}
	// Preserve quoted/complex identifiers (e.g., email-based usernames).
	return trimmed
}

func isSimpleIdentifier(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			continue
		case r >= 'A' && r <= 'Z':
			continue
		case r >= '0' && r <= '9':
			continue
		case r == '_' || r == '$':
			continue
		default:
			return false
		}
	}
	return true
}

func envOrDefault(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
}

// Ensure crypto/rand is linked for JWT signing entropy.
var _ = rand.Reader
