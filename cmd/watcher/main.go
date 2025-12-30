package main

import (
	"log"

	"dockerhub-pull-watcher/internal/app"
)

func main() {
	a, err := app.NewFromEnv()
	if err != nil {
		log.Fatalf("startup: %v", err)
	}
	if err := a.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
