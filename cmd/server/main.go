package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/pankaj/simple-chat/server"
)

func main() {
	host := flag.String("host", getEnvOrDefault("CHAT_HOST", "0.0.0.0"), "Host to listen on")
	port := flag.String("port", getEnvOrDefault("CHAT_PORT", "8080"), "Port to listen on")
	flag.Parse()

	addr := fmt.Sprintf("%s:%s", *host, *port)

	srv := server.New()
	if err := srv.Listen(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
	log.Printf("Chat server listening on %s", addr)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	srv.Shutdown()
}

func getEnvOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
