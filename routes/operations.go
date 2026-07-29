package routes

import (
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/services"
	"github.com/wpmed-videowiki/OWIDImporter/sessions"
	"github.com/wpmed-videowiki/OWIDImporter/utils"
)

type CreateOperationData struct {
	Type  string   `json:"type"`
	Pages []string `json:"pages"` // full urls or bare titles, mixed
}

type ArchiveOperationData struct {
	Archived int `json:"archived"`
}

func getOperationRequestUser(c *gin.Context) *models.User {
	sessionId := c.Request.Header.Get("sessionId")

	if sessionId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session"})
		return nil
	}

	session, ok := sessions.Sessions[sessionId]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown session"})
		return nil
	}
	user, err := models.FindUserByUsername(session.Username)
	if err != nil || user == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown user"})
		return nil
	}

	return user
}

func findOperationForUser(c *gin.Context, user *models.User) *models.Operation {
	operationId := c.Param("id")
	operation, err := models.FindOperationById(operationId)
	if err != nil || operation == nil {
		fmt.Println("Error finding operation: ", err, operation)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown operation"})
		return nil
	}
	if operation.UserId != user.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown operation"})
		return nil
	}
	return operation
}

func CreateOperation(c *gin.Context) {
	user := getOperationRequestUser(c)
	if user == nil {
		return
	}

	var data CreateOperationData
	if err := c.BindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	if models.OperationType(data.Type) != models.OperationTypeUpdateDefaults {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown operation type"})
		return
	}

	titles := make([]string, 0)
	seen := make(map[string]bool)
	invalid := make([]string, 0)
	for _, page := range data.Pages {
		title, err := services.NormalizeCommonsTitle(page)
		if err != nil {
			invalid = append(invalid, page)
			continue
		}
		if !seen[title] {
			seen[title] = true
			titles = append(titles, title)
		}
	}

	if len(invalid) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid pages", "invalid": invalid})
		return
	}
	if len(titles) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No pages provided"})
		return
	}

	operation, err := models.NewOperation(user.ID, models.OperationType(data.Type), "")
	if err != nil {
		fmt.Println("Error creating operation ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error creating operation"})
		return
	}

	for index, title := range titles {
		if _, err := models.NewOperationItem(operation.ID, title, index); err != nil {
			fmt.Println("Error creating operation item ", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "Error creating operation items"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"operationId": operation.ID})
}

func GetOperations(c *gin.Context) {
	user := getOperationRequestUser(c)
	if user == nil {
		return
	}

	queryParams := c.Request.URL.Query()

	archivedStr := queryParams.Get("archived")
	archived := 0
	if archivedStr == "1" {
		archived = 1
	}

	pageStr := queryParams.Get("page")
	page := 1
	if pageStr != "" {
		if num, err := strconv.Atoi(pageStr); err == nil && num > 0 {
			page = num
		}
	}

	perPageStr := queryParams.Get("perPage")
	perPage := 20
	if perPageStr != "" {
		if num, err := strconv.Atoi(perPageStr); err == nil {
			perPage = num
		}
	}

	skip := (page - 1) * perPage

	operations, count, err := models.FindOperationsByUserId(user.ID, archived, skip, perPage)
	if err != nil || operations == nil {
		fmt.Println("Error getting operations: ", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error getting operations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"operations": operations,
		"page":       page,
		"perPage":    perPage,
		"totalPages": int(math.Ceil(float64(count) / float64(perPage))),
	})
}

func GetOperation(c *gin.Context) {
	user := getOperationRequestUser(c)
	if user == nil {
		return
	}

	operation := findOperationForUser(c, user)
	if operation == nil {
		return
	}

	items, err := models.FindOperationItemsByOperationId(operation.ID)
	if err != nil {
		fmt.Println("Error getting operation items: ", err)
	}

	c.JSON(http.StatusOK, gin.H{"operation": operation, "items": items})
}

func CancelOperation(c *gin.Context) {
	user := getOperationRequestUser(c)
	if user == nil {
		return
	}

	operation := findOperationForUser(c, user)
	if operation == nil {
		return
	}

	if operation.Status != models.OperationStatusQueued && operation.Status != models.OperationStatusProcessing {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error cancelling operation"})
		return
	}

	operation.Status = models.OperationStatusCancelled
	if err := operation.Update(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error cancelling operation"})
		return
	}
	utils.SendWSOperation(operation)

	c.JSON(http.StatusOK, gin.H{"operationId": operation.ID})
}

func RetryOperation(c *gin.Context) {
	user := getOperationRequestUser(c)
	if user == nil {
		return
	}

	operation := findOperationForUser(c, user)
	if operation == nil {
		return
	}

	if operation.Status != models.OperationStatusFailed && operation.Status != models.OperationStatusDone && operation.Status != models.OperationStatusCancelled {
		fmt.Println("Error retrying operation: operation with status ", operation.Status)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error retrying operation"})
		return
	}

	models.RequeueFailedOperationItems(operation.ID)
	models.UpdateOperationLastOperationAt(operation.ID)
	operation.Status = models.OperationStatusQueued
	operation.Update()
	utils.SendWSOperation(operation)

	c.JSON(http.StatusOK, gin.H{"operationId": operation.ID})
}

func ArchiveOperation(c *gin.Context) {
	user := getOperationRequestUser(c)
	if user == nil {
		return
	}

	operation := findOperationForUser(c, user)
	if operation == nil {
		return
	}

	if operation.Status == models.OperationStatusProcessing {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot archive a processing operation"})
		return
	}

	var data ArchiveOperationData
	if err := c.BindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Error reading data"})
		return
	}

	operation.Archived = data.Archived
	if err := operation.Update(); err != nil {
		fmt.Println("Error updating operation", err)
	} else {
		utils.SendWSOperation(operation)
	}

	c.JSON(http.StatusOK, gin.H{"operation": operation})
}
