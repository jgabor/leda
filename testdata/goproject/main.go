package main

import (
	"fmt"

	"example.com/testproject/auth"
	"example.com/testproject/server"
)

func main() {
	fmt.Println("starting")
	auth.Init()
	server.Start()
}
