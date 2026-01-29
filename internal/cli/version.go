package cli

// Version information set by ldflags during build
var (
	// Version is the semantic version (set by goreleaser)
	Version = "dev"
	// Commit is the git commit SHA (set by goreleaser)
	Commit = "none"
	// Date is the build date (set by goreleaser)
	Date = "unknown"
)
