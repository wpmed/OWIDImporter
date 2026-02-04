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
			for _, task := range *tasks {
				task.Status = models.TaskStatusFailed
				task.Update()
			}
		}
		time.Sleep(time.Second * 60)
	}
}

func monitorQueuedTasks() {
	for {
		time.Sleep(time.Second * 10)
		count, err := models.FindProcessingTasksCount()
		if err != nil {
			fmt.Println("Error finding processing tasks count", err)
			continue
		}

		if count < 2 {
			task, err := models.FindNextTaskToProcess()
			if err != nil {
				fmt.Println("Error finding next task to process", err)
				continue

			}
			if task == nil {
				// fmt.Println("No next task to process")
				continue
			}
			fmt.Println("Next task: ", task.URL, task.ID)

			user, err := models.FindUserByID(task.UserId)
			if err != nil {
				fmt.Println("Error find next task's user", err)
			}
			if user == nil {
				fmt.Println("Can't find user for the task", task.ID)
			}
			if err != nil || user == nil {
				// Fail the task to get the next
				task.Status = models.TaskStatusFailed
				task.Update()
				continue
			}

			go func() {
				switch task.Type {
				case models.TaskTypeMap:
					fmt.Println("Action message map", task)
					err := services.StartMap(task.ID, user, services.StartData{
						Url:                                  task.URL,
						FileName:                             task.FileName,
						Description:                          task.Description,
						DescriptionOverwriteBehaviour:        task.DescriptionOverwriteBehaviour,
						ImportCountries:                      task.ImportCountries == 1,
						CountryFileName:                      task.CountryFileName,
						CountryDescription:                   task.CountryDescription,
						CountryDescriptionOverwriteBehaviour: task.CountryDescriptionOverwriteBehaviour,
						GenerateTemplateCommons:              task.GenerateTemplateCommons == 1,
						TemplateNameFormat:                   task.CommonsTemplateNameFormat,
					})
					if err != nil {
						log.Println("Error starting map", err)
					}
				case models.TaskTypeChart:
					fmt.Println("Action message chart", task)
					err := services.StartChart(task.ID, user, services.StartData{
						Url:                           task.URL,
						FileName:                      task.FileName,
						Description:                   task.Description,
						DescriptionOverwriteBehaviour: task.DescriptionOverwriteBehaviour,
					})
					if err != nil {
						log.Println("Error starting map", err)
					}
				}
			}()
		}

	}
}
