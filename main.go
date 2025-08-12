package main

import (
	"fmt"
	"log"
	"time"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/joho/godotenv"

	"github.com/wpmed-videowiki/OWIDImporter/env"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/routes"
)

func main() {
	err := godotenv.Load(".env")
	models.Init()
	if err != nil {
		log.Println("Failed to load environment variables: ", err)
	}
	// Verify environment variables
	env.GetEnv()

	go func() {
		monitorStalledTasks()
	}()

	// Download browser if not available
	b := launcher.NewBrowser()
	b.RootDir = "/workspace/.cache/rod/browser"
	fmt.Println("Dir is", b.Dir(), b.RootDir)
	b.Hosts = []launcher.Host{launcher.HostNPM, launcher.HostPlaywright}
	r, err := b.Get()
	fmt.Println("launcher ", r, err)

	router := routes.BuildRoutes()
	err = router.Run(":8000")
	if err != nil {
		log.Fatalf("Failed to run router: %v", err)
	}
}

func monitorStalledTasks() {
	for {
		tasks, err := models.FindStalledTasks()
		fmt.Println("Found stalled tasks", len(*tasks), err)
		for _, task := range *tasks {
			task.Status = models.TaskStatusFailed
			task.Update()
		}
		time.Sleep(time.Second * 60)
	}
}
