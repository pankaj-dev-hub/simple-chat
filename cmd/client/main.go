package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/pankaj/simple-chat/client"
)

func main() {
	host := flag.String("host", getEnvOrDefault("CHAT_HOST", "localhost"), "Server host")
	port := flag.String("port", getEnvOrDefault("CHAT_PORT", "8080"), "Server port")
	username := flag.String("username", getEnvOrDefault("CHAT_USERNAME", ""), "Username")
	flag.Parse()

	if *username == "" {
		fmt.Fprintln(os.Stderr, "Username is required. Use -username flag or CHAT_USERNAME env var.")
		os.Exit(1)
	}

	addr := fmt.Sprintf("%s:%s", *host, *port)
	c, err := client.New(addr, *username)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer c.Close()

	fmt.Printf("Connected to %s as %s\n", addr, *username)
	fmt.Println("Commands: 'send <message>' or 'leave'")
	c.Run()
}

func getEnvOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
