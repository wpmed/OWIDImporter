package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// OWID exports an SVG by creating a blob URL and clicking an <a download> for it. Instead of
// letting the browser download it (the download path and events are browser-wide, so windows
// sharing a browser could catch each other's files), the page keeps the SVG blob and its file
// name, and Go reads them back. Other downloads (PNG, CSV) are left alone.

// svgCaptureJS is installed before the first navigation and runs again on every new document.
const svgCaptureJS = `(() => {
  if (window.__owidCap) return;
  const cap = window.__owidCap = { blobs: new Map(), results: [] };
  const origCreate = URL.createObjectURL;
  URL.createObjectURL = function (obj) {
    const url = origCreate.apply(this, arguments);
    if (obj instanceof Blob && /^image\/svg\+xml/i.test(obj.type)) cap.blobs.set(url, obj);
    return url;
  };
  const origClick = HTMLAnchorElement.prototype.click;
  HTMLAnchorElement.prototype.click = function () {
    const blob = this.hasAttribute('download') ? cap.blobs.get(this.href) : undefined;
    if (blob) {
      cap.blobs.delete(this.href);
      cap.results.push({ name: this.getAttribute('download'), blob });
      return;
    }
    return origClick.apply(this, arguments);
  };
})()`

// svgCaptureResetJS drops results and blobs left by an earlier export, so the next result is this click's.
const svgCaptureResetJS = `() => {
  const c = window.__owidCap;
  if (!c) return false;
  c.results.length = 0;
  c.blobs.clear();
  return true;
}`

// svgCaptureTakeJS returns the oldest captured SVG as {name, text}, or null when there is none yet.
const svgCaptureTakeJS = `async () => {
  const c = window.__owidCap;
  if (!c) return { missing: true };
  const r = c.results.shift();
  if (!r) return null;
  return { name: r.name || '', text: await r.blob.text() };
}`

// svgCapturePoll is how often captureOWIDSVG looks for the export.
const svgCapturePoll = 100 * time.Millisecond

// svgCaptureTimeout bounds waiting for an export after clicking the SVG button (map and country charts).
var svgCaptureTimeout = 90 * time.Second

// installSVGCapture hooks page so OWID's SVG exports are kept in the page. Call it before the
// first navigation.
func installSVGCapture(page *rod.Page) error {
	if _, err := page.EvalOnNewDocument(svgCaptureJS); err != nil {
		return fmt.Errorf("installing SVG capture: %w", err)
	}
	return nil
}

// captureOWIDSVG clears earlier captures, runs click (which clicks the modal's SVG button) and
// waits until the export arrives, timeout passes or the page's context ends. It returns OWID's
// file name and the SVG bytes. click must make its page calls with ctx: OWID exports inside the
// click handler, so a page that hangs while exporting would otherwise block the click forever.
func captureOWIDSVG(page *rod.Page, timeout time.Duration, click func(ctx context.Context) error) (string, []byte, error) {
	timed := page.Timeout(timeout)
	defer timed.CancelTimeout()
	ctx := timed.GetContext()
	res, err := timed.Eval(svgCaptureResetJS)
	if err != nil {
		return "", nil, fmt.Errorf("preparing SVG capture: %w", err)
	}
	if !res.Value.Bool() {
		return "", nil, fmt.Errorf("SVG capture is not installed in this page")
	}
	if err := click(ctx); err != nil {
		return "", nil, fmt.Errorf("error clicking SVG download button: %w", err)
	}
	for {
		res, err := timed.Eval(svgCaptureTakeJS)
		if err != nil {
			if ctx.Err() != nil {
				return "", nil, fmt.Errorf("timeout waiting for SVG download: %w", ctx.Err())
			}
			return "", nil, fmt.Errorf("reading SVG download: %w", err)
		}
		if v := res.Value; !v.Nil() {
			if v.Get("missing").Bool() {
				// The page navigated to a document without the hook
				return "", nil, fmt.Errorf("SVG capture is not installed in this page")
			}
			return v.Get("name").Str(), []byte(v.Get("text").Str()), nil
		}
		select {
		case <-ctx.Done():
			return "", nil, fmt.Errorf("timeout waiting for SVG download: %w", ctx.Err())
		case <-time.After(svgCapturePoll):
		}
	}
}

// capturedSVGName is the on-disk name for a captured export: OWID's name without any path.
func capturedSVGName(name string) string {
	name = filepath.Base(name)
	if name == "" || name == "." || name == ".." || name == "/" {
		return "chart.svg"
	}
	return name
}

// captureOWIDSVGToDir captures an export (see captureOWIDSVG) and writes it into dir under
// OWID's file name, creating dir. It returns the file's name.
func captureOWIDSVGToDir(page *rod.Page, dir string, timeout time.Duration, click func(ctx context.Context) error) (string, error) {
	name, data, err := captureOWIDSVG(page, timeout, click)
	if err != nil {
		return "", err
	}
	return writeCapturedSVG(dir, name, data)
}

// clickElement returns a capture click (see captureOWIDSVG) that clicks el like Element.Click,
// but bounded by ctx. rod's Mouse always sends its events with the page's original context
// (Page.Context and Element.Context don't reach it), so Element.Click can't be bounded.
func clickElement(el *rod.Element) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		el := el.Context(ctx)
		pt, err := el.WaitInteractable()
		if err != nil {
			return err
		}
		if err := el.WaitEnabled(); err != nil {
			return err
		}
		page := el.Page().Context(ctx)
		left, down, up := proto.InputMouseButtonLeft, 1, 0
		for _, e := range []proto.InputDispatchMouseEvent{
			{Type: proto.InputDispatchMouseEventTypeMouseMoved, X: pt.X, Y: pt.Y},
			{Type: proto.InputDispatchMouseEventTypeMousePressed, X: pt.X, Y: pt.Y, Button: left, Buttons: &down, ClickCount: 1},
			{Type: proto.InputDispatchMouseEventTypeMouseReleased, X: pt.X, Y: pt.Y, Button: left, Buttons: &up, ClickCount: 1},
		} {
			if err := e.Call(page); err != nil {
				return err
			}
		}
		return nil
	}
}

// writeCapturedSVG writes data into dir under OWID's file name, creating dir.
func writeCapturedSVG(dir, name string, data []byte) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	name = capturedSVGName(name)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
		return "", err
	}
	return name, nil
}
