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
	"github.com/wpmed-videowiki/OWIDImporter/services"
	"github.com/wpmed-videowiki/OWIDImporter/utils"
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

	go func() {
		monitorQueuedTasks()
	}()

	go func() {
		monitorStalledOperations()
	}()

	go func() {
		monitorQueuedOperations()
	}()

	// Download browser if not available
	// b := launcher.NewBrowser()
	// fmt.Println("Dir is", b.Dir(), b.RootDir)
	// b.Hosts = []launcher.Host{launcher.HostNPM, launcher.HostPlaywright}
	// r, err := b.Get()
	// fmt.Println("launcher ", r, err)

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
			for _, task := range *tasks {
				task.Status = models.TaskStatusFailed
				task.Update()
				utils.SendWSTask(&task)
			}
		}
		time.Sleep(time.Second * 60)
	}
}

func monitorStalledOperations() {
	for {
		operations, err := models.FindStalledOperations()
		if operations != nil && len(*operations) > 0 {
			fmt.Println("Found stalled operations", len(*operations), err)
			for _, operation := range *operations {
				operation.Status = models.OperationStatusFailed
				operation.Update()
				utils.SendWSOperation(&operation)
			}
		}
		time.Sleep(time.Second * 60)
	}
}

func monitorQueuedOperations() {
	for {
		time.Sleep(time.Second * 10)
		count, err := models.FindProcessingOperationsCount()
		if err != nil {
			fmt.Println("Error finding processing operations count", err)
			continue
		}

		if count < 1 {
			operation, err := models.FindNextOperationToProcess()
			if err != nil {
				fmt.Println("Error finding next operation to process", err)
				continue
			}
			if operation == nil {
				continue
			}
			fmt.Println("Next operation: ", operation.Type, operation.ID)

			user, err := models.FindUserByID(operation.UserId)
			if err != nil || user == nil {
				fmt.Println("Can't find user for the operation", operation.ID, err)
				// Fail the operation to get the next
				operation.Status = models.OperationStatusFailed
				operation.Update()
				utils.SendWSOperation(operation)
				continue
			}

			go func() {
				if err := services.StartOperation(operation.ID, user); err != nil {
					log.Println("Error starting operation", err)
				}
			}()
		}

	}
}

// Maps run one at a time; iframe imports run OWID_IFRAME_CONCURRENCY at a time
func monitorQueuedTasks() {
	limits := map[models.TaskType]int{
		models.TaskTypeMap:    1,
		models.TaskTypeIframe: env.GetEnv().OWID_IFRAME_CONCURRENCY,
	}
	services.NewTaskQueue(limits, services.RunTask).Run(services.TaskQueuePollInterval)
}
