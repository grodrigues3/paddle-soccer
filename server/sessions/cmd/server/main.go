// Server binary for session management
package main

import (
	"log"
	"os"

	"github.com/markmandel/paddle-soccer/sessions"
)

const (
	// port to listen on
	portEnv = "PORT"
	// address to listen to redis on
	redisAddressEnv = "REDIS_SERVICE"
)

func main() {
	// get environment variables
	port := os.Getenv(portEnv)
	// default for port
	if port == "" {
		port = "8080"
	}
	log.Print("[Info][Main] Creating server...")
	s := sessions.NewServer(":"+port, os.Getenv(redisAddressEnv))
	s.Start()
}