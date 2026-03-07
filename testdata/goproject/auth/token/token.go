package token

// Validate checks if a token is valid.
func Validate(t string) bool {
	return t != ""
}

// Generate creates a new token.
func Generate(userID string) string {
	return "tok_" + userID
}
