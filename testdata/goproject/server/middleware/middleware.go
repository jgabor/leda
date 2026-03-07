package middleware

import "example.com/testproject/auth"

// Apply sets up middleware chain.
func Apply() {
	auth.Authenticate("", "")
}

// Logger logs requests.
func Logger() {}
