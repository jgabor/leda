package server

import "example.com/testproject/server/middleware"

// Start begins the server.
func Start() {
	middleware.Apply()
}
