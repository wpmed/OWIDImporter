package services

import (
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/wpmed-videowiki/OWIDImporter/models"
	"github.com/wpmed-videowiki/OWIDImporter/utils"
)

func FailTaskProcess(taskProcess *models.TaskProcess) {
	taskProcess.Status = models.TaskProcessStatusFailed
	taskProcess.Update()
	utils.SendWSTaskProcess(taskProcess.TaskId, taskProcess)
}

var (
	// downloadDialogCloseTimeout bounds closing the download dialog (a var so tests can shorten it).
	downloadDialogCloseTimeout = 10 * time.Second
	// downloadDialogGoneWait is how long one close attempt waits for the dialog to go.
	downloadDialogGoneWait = time.Second
)

// CloseDownloadPopup closes the download dialog, if it is open, and checks it is gone. It
// tries the close button, then Escape, then a script click on the close button, in turn,
// until the dialog is gone or downloadDialogCloseTimeout passes. A dialog that stays open is
// an errPageNotReady error: it keeps the focus, so the timeline ignores the keys that move
// the year, and reloading the page's current URL (?overlay=download-vis) opens it again.
func CloseDownloadPopup(page *rod.Page) error {
	p := page.Timeout(downloadDialogCloseTimeout)
	defer p.CancelTimeout()
	tried := false
	for attempt := 0; ; attempt++ {
		open, _, err := p.Has(DOWNLOAD_POPUP_SELECTOR)
		if err != nil {
			if tried {
				fmt.Println("ERROR the download dialog did not close:", err)
				return fmt.Errorf("the download dialog stayed open: %v: %w", err, errPageNotReady)
			}
			return fmt.Errorf("checking the download dialog: %v: %w", err, errPageNotReady)
		}
		if !open {
			return nil
		}
		tried = true
		closeDownloadDialogOnce(p, attempt%3)
		waitDownloadDialogGone(p, downloadDialogGoneWait)
	}
}

// closeDownloadDialogOnce makes one attempt to close the download dialog: 0 clicks the close
// button, 1 presses Escape, 2 clicks the close button from a script. Errors are left to the
// caller's check that the dialog is gone.
func closeDownloadDialogOnce(p *rod.Page, how int) {
	ctx := p.GetContext()
	switch how {
	case 0:
		if has, el, err := p.Has(DOWNLOAD_POPUP_CLOSE_BUTTON); err == nil && has {
			_ = clickElement(el)(ctx)
		}
	case 1:
		// Sent with p's deadline: rod's Keyboard always uses the page's original context
		for _, t := range []proto.InputDispatchKeyEventType{proto.InputDispatchKeyEventTypeKeyDown, proto.InputDispatchKeyEventTypeKeyUp} {
			_ = input.Escape.Encode(t, 0).Call(p)
		}
	default:
		_, _ = p.Eval(`(sel) => { const b = document.querySelector(sel); if (b) b.click(); }`, DOWNLOAD_POPUP_CLOSE_BUTTON)
	}
}

// waitDownloadDialogGone waits up to d for the download dialog to leave the page.
func waitDownloadDialogGone(p *rod.Page, d time.Duration) {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if open, _, err := p.Has(DOWNLOAD_POPUP_SELECTOR); err != nil || !open {
			return
		}
		select {
		case <-p.GetContext().Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}
