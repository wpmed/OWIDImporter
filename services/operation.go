package services

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/wpmed-videowiki/OWIDImporter/env"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/utils"
)

const OPERATION_EDIT_DELAY = 2 * time.Second

// NormalizeCommonsTitle turns a wiki page reference - either a full URL
// (https://commons.wikimedia.org/wiki/Template:OWID/Foo or
// /w/index.php?title=...) or a bare title - into a normalized page title.
func NormalizeCommonsTitle(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("empty page reference")
	}

	title := input
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		parsed, err := url.Parse(input)
		if err != nil {
			return "", fmt.Errorf("invalid url: %s", input)
		}
		if idx := strings.Index(parsed.Path, "/wiki/"); idx != -1 {
			title = parsed.Path[idx+len("/wiki/"):]
		} else if strings.HasSuffix(parsed.Path, "/index.php") && parsed.Query().Get("title") != "" {
			title = parsed.Query().Get("title")
		} else if idx := strings.Index(parsed.Path, "/index.php/"); idx != -1 {
			title = parsed.Path[idx+len("/index.php/"):]
		} else {
			return "", fmt.Errorf("unsupported wiki url: %s", input)
		}
	}

	unescaped, err := url.PathUnescape(title)
	if err == nil {
		title = unescaped
	}

	title = strings.ReplaceAll(title, "_", " ")
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		return "", fmt.Errorf("empty page title: %s", input)
	}

	return title, nil
}

func StartOperation(operationId string, user *models.User) error {
	operation, err := models.FindOperationById(operationId)
	if err != nil {
		return err
	}

	operation.Status = models.OperationStatusProcessing
	if err := operation.Update(); err != nil {
		fmt.Println("Error setting operation to Processing: ", err)
	}
	models.UpdateOperationLastOperationAt(operation.ID)
	utils.SendWSOperation(operation)

	// Requeue items left processing/failed by a previous run (retry/crash recovery)
	if err := models.RequeueFailedOperationItems(operation.ID); err != nil {
		fmt.Println("Error requeueing operation items: ", err)
	}

	items, err := models.FindOperationItemsByOperationId(operation.ID)
	if err != nil {
		operation.Status = models.OperationStatusFailed
		operation.Update()
		utils.SendWSOperation(operation)
		return err
	}

	for i := range items {
		item := &items[i]
		if item.Status != models.OperationItemStatusQueued {
			continue
		}

		if err := operation.Reload(); err == nil && operation.Status == models.OperationStatusCancelled {
			utils.SendWSOperation(operation)
			return nil
		}

		item.Status = models.OperationItemStatusProcessing
		item.Update()
		utils.SendWSOperationItem(operation.ID, item)

		var result string
		var itemErr error
		switch operation.Type {
		case models.OperationTypeUpdateDefaults:
			result, itemErr = UpdateTemplateWithDefaults(user, env.GetEnv().OWID_MW_API, item.Title)
		default:
			itemErr = fmt.Errorf("unknown operation type: %s", operation.Type)
		}

		if itemErr != nil {
			item.Status = models.OperationItemStatusFailed
			item.Error = itemErr.Error()
		} else if result == "nochange" {
			item.Status = models.OperationItemStatusSkipped
		} else {
			item.Status = models.OperationItemStatusUpdated
		}
		item.Update()
		utils.SendWSOperationItem(operation.ID, item)

		models.UpdateOperationLastOperationAt(operation.ID)
		time.Sleep(OPERATION_EDIT_DELAY)
	}

	operation.Status = models.OperationStatusDone
	operation.Update()
	utils.SendWSOperation(operation)

	return nil
}
