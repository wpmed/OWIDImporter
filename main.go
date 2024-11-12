package main

import (
	"fmt"
	"log"

	"github.com/joho/godotenv"

	"github.com/wpmed-videowiki/OWIDImporter/env"
	"github.com/wpmed-videowiki/OWIDImporter/routes"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatalf("Failed to load environment variables: %v", err)
	}
	// Verify environment variables
	env := env.GetEnv()
	fmt.Println(env)
	router := routes.BuildRoutes()

	err = router.Run(":8000")
	if err != nil {
		log.Fatalf("Failed to run router: %v", err)
	}
}
