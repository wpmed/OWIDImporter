package routes

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/services"
	"github.com/wpmed-videowiki/OWIDImporter/sessions"
)

type CreateTaskData struct {
	Action                               string                               `json:"action"`
	Url                                  string                               `json:"url"`
	FileName                             string                               `json:"fileName"`
	Description                          string                               `json:"description"`
	DescriptionOverwriteBehaviour        models.DescriptionOverwriteBehaviour `json:"descriptionOverwriteBehaviour"`
	ImportCountries                      bool                                 `json:"importCountries"`
	CountryFileName                      string                               `json:"countryFileName"`
	CountryDescription                   string                               `json:"countryDescription"`
	CountryDescriptionOverwriteBehaviour models.DescriptionOverwriteBehaviour `json:"countryDescriptionOverwriteBehaviour"`
	GenerateTemplateCommons              bool                                 `json:"generateTemplateCommons"`
	ChartParameters                      string                               `json:"chartParameters"`    // query string for the chart params
	TemplateNameFormat                   string                               `json:"templateNameFormat"` // formatting for OWID Template name
}

type GetTaskResponse struct {
	Task      models.Task          `json:"task"`
	Processes []models.TaskProcess `json:"processes"`
	WikiText  string               `json:"wikiText"`
}

func CreateTask(c *gin.Context) {
	sessionId := c.Request.Header.Get("sessionId")

	if sessionId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session"})
		return
	}

	session, ok := sessions.Sessions[sessionId]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown session"})
		return
	}
	user, err := models.FindUserByUsername(session.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown user"})
		return
	}

	var data CreateTaskData
	if err := c.BindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	var modelType models.TaskType
	switch data.Action {
	case "startMap":
		modelType = models.TaskTypeMap
	case "startChart":
		modelType = models.TaskTypeChart
	}

	importCountries := 0
	if data.ImportCountries {
		importCountries = 1
	}

	generateTemplateCommons := 0
	if data.GenerateTemplateCommons {
		generateTemplateCommons = 1
	}

	task, err := models.NewTask(
		user.ID,
		data.Url,
		data.FileName,
		data.Description,
		data.DescriptionOverwriteBehaviour,
		"",
		models.TaskStatusQueued,
		modelType,
		importCountries,
		data.CountryFileName,
		data.CountryDescription,
		data.CountryDescriptionOverwriteBehaviour,
		generateTemplateCommons,
		data.ChartParameters,
		data.TemplateNameFormat,
	)
	if err != nil {
		fmt.Println("Error creating task ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error creating task"})
		return
	}

	// go func() {
	// 	switch task.Type {
	// 	case models.TaskTypeMap:
	// 		fmt.Println("Action message map", task)
	// 		err := services.StartMap(task.ID, user, services.StartData{
	// 			Url:                                  task.URL,
	// 			FileName:                             task.FileName,
	// 			Description:                          task.Description,
	// 			DescriptionOverwriteBehaviour:        task.DescriptionOverwriteBehaviour,
	// 			ImportCountries:                      task.ImportCountries == 1,
	// 			CountryFileName:                      task.CountryFileName,
	// 			CountryDescription:                   task.CountryDescription,
	// 			CountryDescriptionOverwriteBehaviour: task.CountryDescriptionOverwriteBehaviour,
	// 			GenerateTemplateCommons:              task.GenerateTemplateCommons == 1,
	// 			TemplateNameFormat:                   task.CommonsTemplateNameFormat,
	// 		})
	// 		if err != nil {
	// 			log.Println("Error starting map", err)
	// 		}
	// 	case models.TaskTypeChart:
	// 		fmt.Println("Action message chart", task)
	// 		err := services.StartChart(task.ID, user, services.StartData{
	// 			Url:                           task.URL,
	// 			FileName:                      task.FileName,
	// 			Description:                   task.Description,
	// 			DescriptionOverwriteBehaviour: task.DescriptionOverwriteBehaviour,
	// 		})
	// 		if err != nil {
	// 			log.Println("Error starting map", err)
	// 		}
	// 	}
	// }()

	c.JSON(http.StatusOK, gin.H{"taskId": task.ID})
}

func GetTask(c *gin.Context) {
	taskId := c.Param("id")
	task, err := models.FindTaskById(taskId)
	if err != nil || task == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot find task"})
		return
	}
	processes, err := models.FindTaskProcessesByTaskId(taskId)
	if err != nil {
		fmt.Println("Error getting task processes: ", err)
	}

	res := GetTaskResponse{
		Task:      *task,
		Processes: processes,
		WikiText:  "",
	}
	if task.Status == models.TaskStatusDone {
		switch task.Type {
		case models.TaskTypeMap:
			text, err := services.GetMapTemplate(task.ID)
			if err != nil {
				fmt.Println("Error getting task wikitext", taskId, err)
			}
			res.WikiText = text
		case models.TaskTypeChart:
			text, err := services.GetChartTemplate(task.ID)
			if err != nil {
				fmt.Println("Error getting task wikitext", taskId, err)
			}
			res.WikiText = text
		}
	}

	c.JSON(http.StatusOK, res)
}

func GetTasks(c *gin.Context) {
	sessionId := c.Request.Header.Get("sessionId")

	if sessionId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session"})
		return
	}

	session, ok := sessions.Sessions[sessionId]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown session"})
		return
	}
	user, err := models.FindUserByUsername(session.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown user"})
		return
	}
	queryParams := c.Request.URL.Query()
	taskType := queryParams.Get("taskType")

	tasks, err := models.FindTaskByUserId(user.ID, taskType)
	if err != nil || tasks == nil {
		c.JSON(http.StatusBadRequest, make([]string, 0))
		return
	}

	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

func RetryTask(c *gin.Context) {
	sessionId := c.Request.Header.Get("sessionId")

	if sessionId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session"})
		return
	}

	session, ok := sessions.Sessions[sessionId]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown session"})
		return
	}
	user, err := models.FindUserByUsername(session.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown user"})
		return
	}

	taskId := c.Param("id")
	task, err := models.FindTaskById(taskId)
	if err != nil || task == nil {
		fmt.Println("Error retrying task: ", err, task)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error retrying task"})
		return
	}

	if task.Status != models.TaskStatusFailed && task.Status != models.TaskStatusDone {
		fmt.Println("Error retrying task: task with status ", task.Status)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error retrying task"})
		return
	}
	models.FailProcessingTaskProcesses(task.ID)
	models.UpdateTaskLastOperationAt(task.ID)
	task.Status = models.TaskStatusQueued
	task.Update()

	// go func() {
	// 	switch task.Type {
	// 	case models.TaskTypeMap:
	// 		fmt.Println("Action message map", task)
	// 		err := services.StartMap(task.ID, user, services.StartData{
	// 			Url:                                  task.URL,
	// 			FileName:                             task.FileName,
	// 			Description:                          task.Description,
	// 			DescriptionOverwriteBehaviour:        task.DescriptionOverwriteBehaviour,
	// 			ImportCountries:                      task.ImportCountries == 1,
	// 			CountryFileName:                      task.CountryFileName,
	// 			CountryDescription:                   task.CountryDescription,
	// 			CountryDescriptionOverwriteBehaviour: task.CountryDescriptionOverwriteBehaviour,
	// 			GenerateTemplateCommons:              task.GenerateTemplateCommons == 1,
	// 			TemplateNameFormat:                   task.CommonsTemplateNameFormat,
	// 		})
	// 		if err != nil {
	// 			log.Println("Error starting map", err)
	// 		}
	// 	case models.TaskTypeChart:
	// 		fmt.Println("Action message chart", task)
	// 		err := services.StartChart(task.ID, user, services.StartData{
	// 			Url:                           task.URL,
	// 			FileName:                      task.FileName,
	// 			Description:                   task.Description,
	// 			DescriptionOverwriteBehaviour: task.DescriptionOverwriteBehaviour,
	// 		})
	// 		if err != nil {
	// 			log.Println("Error starting map", err)
	// 		}
	// 	}
	// }()

	c.JSON(http.StatusOK, gin.H{"taskId": task.ID})
}

func CancelTask(c *gin.Context) {
	sessionId := c.Request.Header.Get("sessionId")

	if sessionId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session"})
		return
	}

	session, ok := sessions.Sessions[sessionId]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown session"})
		return
	}
	user, err := models.FindUserByUsername(session.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown user"})
		return
	}

	taskId := c.Param("id")
	task, err := models.FindTaskById(taskId)
	if err != nil || task == nil {
		fmt.Println("Error retrying task: ", err, task)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error retrying task"})
		return
	}
	task.Status = models.TaskStatusFailed
	if err := task.Update(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error stopping task"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"taskId": task.ID})
}

// func GenerateCommonsTemplate(c *gin.Context) {
// 	sessionId := c.Request.Header.Get("sessionId")
//
// 	if sessionId == "" {
// 		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session"})
// 		return
// 	}
//
// 	session, ok := sessions.Sessions[sessionId]
// 	if !ok {
// 		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown session"})
// 		return
// 	}
// 	user, err := models.FindUserByUsername(session.Username)
// 	if err != nil || user == nil {
// 		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown user"})
// 		return
// 	}
//
// 	taskId := c.Param("id")
// 	task, err := models.FindTaskById(taskId)
// 	if err != nil || task == nil {
// 		fmt.Println("Error retrying task: ", err, task)
// 		c.JSON(http.StatusBadRequest, gin.H{"error": "Error retrying task"})
// 		return
// 	}
//
// 	services.CreateCommonsTemplatePage(taskId, user)
// 	c.JSON(http.StatusOK, gin.H{"taskId": task.ID})
// }
