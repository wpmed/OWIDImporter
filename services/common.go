package services

import (
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/utils"
)

func FailTaskProcess(taskProcess *models.TaskProcess) {
	taskProcess.Status = models.TaskProcessStatusFailed
	taskProcess.Update()
	utils.SendWSTaskProcess(taskProcess.TaskId, taskProcess)
}

func CloseDownloadPopup(page *rod.Page) {
	if err := utils.WaitElementWithTimeout(page, DOWNLOAD_POPUP_CLOSE_BUTTON, time.Millisecond*50); err != nil {
		return
	}

	closeBtn := page.MustElement(DOWNLOAD_POPUP_CLOSE_BUTTON)
	if closeBtn != nil {
		closeBtn.Click(proto.InputMouseButtonLeft, 1)
	}
}
