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
	e := env.GetEnv()
	if e.OWID_ROD_BROWSER_DIR != "" {
		launcher.DefaultBrowserDir = e.OWID_ROD_BROWSER_DIR // "/workspace/.cache/rod/browser"
	}
	go func() {
		monitorStalledTasks()
	}()

	// Download browser if not available
	b := launcher.NewBrowser()
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
		if tasks != nil && len(*tasks) > 0 {
			fmt.Println("Found stalled tasks", len(*tasks), err)
		}
		for _, task := range *tasks {
			task.Status = models.TaskStatusFailed
			task.Update()
		}
		time.Sleep(time.Second * 60)
	}
}
