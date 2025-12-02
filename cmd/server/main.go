package main

import (
	"log"
	"repello/internal/api"
	"repello/internal/matching"
	"repello/internal/metrics"
)

func main() {
	m := metrics.NewMetrics()
	engine := matching.NewEngine(m)
	server := api.NewAPIServer(":8080", engine, m)

	log.Println("Server starting on port 8080...")
	if err := server.Run(); err != nil {
		log.Fatalf("could not start server: %s\n", err)
	}
}
