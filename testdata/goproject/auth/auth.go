package auth

import "example.com/testproject/auth/token"

// Init initializes the auth system.
func Init() {
	token.Validate("")
}

// Authenticate checks user credentials.
func Authenticate(user, pass string) bool {
	return user != "" && pass != ""
}
