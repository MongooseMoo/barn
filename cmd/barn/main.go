package main

import (
	"barn/server"
	"flag"
	"log"
)

func main() {
	dbPath := flag.String("db", "Test.db", "Database file path")
	port := flag.Int("port", 7777, "Listen port")
	flag.Parse()

	log.Printf("Barn MOO Server")
	log.Printf("Database: %s", *dbPath)
	log.Printf("Port: %d", *port)

	srv, err := server.NewServer(*dbPath, *port)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	if err := srv.LoadDatabase(); err != nil {
		log.Fatalf("Failed to load database: %v", err)
	}

	log.Printf("Starting server on port %d...", *port)
	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
