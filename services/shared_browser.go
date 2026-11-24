package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
)

// All browser work (map and iframe tasks, the /iframe/info lookup, the chart parameter routes)
// shares one long-lived browser instead of launching one per worker or request: a cold browser
// spends ~3s downloading OWID's scripts, a warm one with a filled cache ~0.9s, and each extra
// browser costs a few hundred MB.
//
// Every worker gets its own window, not a tab: background tabs of one window are hidden, run no
// requestAnimationFrame, and OWID does not render in them, so tabs would run one after another.
// Windows are all visible and render in parallel.

var (
	// browserIdleTimeout closes the shared browser after this long with no window in use (a var so tests can shorten it).
	browserIdleTimeout = 10 * time.Minute
	// browserHealthTimeout bounds the liveness check run before handing out the browser.
	browserHealthTimeout = 3 * time.Second
	// browserShutdownTimeout bounds closing the browser and waiting for its process to exit.
	browserShutdownTimeout = 5 * time.Second
	// browserWatchInterval is how often the watchdog checks the browser still answers. Workers
	// blocked in a call to a hung browser never notice it themselves; killing it frees them.
	// A browser that stops (SIGSTOP) is killed and relaunched about 28-34 s later (measured):
	// two missed checks, plus the 2-probe health check before the relaunch.
	browserWatchInterval = 10 * time.Second
	// windowCloseTimeout bounds closing one window. Page.Close holds a browser-wide lock in rod,
	// so a window that never closes must not block every other worker.
	windowCloseTimeout = 5 * time.Second
	// windowWatchInterval and windowHealthTimeout drive each worker window's hang watchdog: a
	// window whose page misses two checks in a row (its renderer stuck in a script; closed about
	// 20-30 s after it hangs) is closed, which ends the worker's blocked calls on it, and its
	// item is retried.
	windowWatchInterval = 10 * time.Second
	windowHealthTimeout = 5 * time.Second
	// windowLoadTimeout bounds loading a chart page in a worker's window.
	windowLoadTimeout = 2 * time.Minute
	// crashLoopMaxRelaunches relaunches after a dead browser are allowed within crashLoopWindow;
	// past that the browser keeps dying, so work fails with errBrowserCrashLoop instead of spinning.
	crashLoopMaxRelaunches = 3
	crashLoopWindow        = 2 * time.Minute
	// launchSharedBrowser starts the shared browser and returns it, its keeper tab and a stop
	// func that ends its process and releases its profile; stop(true) kills a dead or hung
	// browser's whole process group without a graceful close (a var so tests can substitute it).
	launchSharedBrowser = launchSharedBrowserDefault
	// sharedBrowserLauncher builds the launcher of the shared browser (a var so tests can pick the browser).
	sharedBrowserLauncher = newBrowserLauncher
)

// browserCrashRetries is how many times a worker reruns its item after the browser died under it.
const browserCrashRetries = 2

// windowStuckRetries is how many times in a row (without an item finishing in between) a
// worker replaces its stuck window and retries the item.
const windowStuckRetries = 1

var errBrowserCrashLoop = errors.New("the browser keeps crashing; not relaunching it for now")

// errWindowStuck marks a step that failed because the worker's window stopped answering.
var errWindowStuck = errors.New("the browser window stopped answering")

// errPageNotReady marks a step that failed because the page, while still answering, did not
// load in time or did not show what the step needs (the download button, the timeline). Like
// a stuck window, it is retried once in a fresh window.
var errPageNotReady = errors.New("the chart page did not load or show its controls in time")

// sharedBrowser is the shared browser and its bookkeeping, guarded by mu.
type sharedBrowser struct {
	mu      sync.Mutex
	browser *rod.Browser
	// stop ends the browser process and releases its profile.
	stop func(dead bool)
	// keeper is a blank tab that is never closed, so closing the last window can never
	// end the browser (a headful browser quits when its last window closes).
	keeper *rod.Page
	// gen identifies the current browser; a holder of an older gen knows it was replaced.
	gen int
	// inUse counts holders (one per open window or running lookup); idle close waits for 0.
	inUse     int
	idleTimer *time.Timer
	// watchStop ends the current browser's watchdog.
	watchStop chan struct{}
	// deaths are when browsers were found dead, for the crash-loop guard.
	deaths []time.Time
}

var theSharedBrowser = &sharedBrowser{}

// acquire returns the shared browser, launching it (or replacing a dead one) when needed.
// Every acquire must be paired with a release. Holders of the same dead browser all end up in
// the one replacement: the first to get here replaces it, the rest find the new one alive.
func (sb *sharedBrowser) acquire() (*rod.Browser, int, error) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if sb.idleTimer != nil {
		sb.idleTimer.Stop()
		sb.idleTimer = nil
	}
	if sb.browser != nil && !sb.aliveLocked() {
		fmt.Println("[browser] shared browser not responding, replacing it")
		sb.diedLocked()
	}
	if sb.browser == nil {
		if sb.crashLoopLocked() {
			fmt.Printf("[browser] browser died %d times in %s, not relaunching it for now\n", len(sb.deaths), crashLoopWindow)
			return nil, 0, errBrowserCrashLoop
		}
		browser, keeper, stop, err := launchSharedBrowser()
		if err != nil {
			return nil, 0, fmt.Errorf("cannot start browser: %w", err)
		}
		sb.browser, sb.keeper, sb.stop = browser, keeper, stop
		sb.gen++
		sb.watchStop = make(chan struct{})
		go sb.watch(sb.gen, browser, keeper, sb.watchStop, browserWatchInterval, browserHealthTimeout)
	}
	sb.inUse++
	return sb.browser, sb.gen, nil
}

// release ends one hold. dead replaces the browser (when it is still the current one);
// the last holder out starts the idle timer.
func (sb *sharedBrowser) release(gen int, dead bool) {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	sb.inUse--
	if dead && gen == sb.gen && sb.browser != nil {
		sb.diedLocked()
	}
	if sb.inUse == 0 && sb.browser != nil {
		current := sb.gen
		sb.idleTimer = time.AfterFunc(browserIdleTimeout, func() {
			sb.mu.Lock()
			defer sb.mu.Unlock()
			if sb.inUse == 0 && sb.browser != nil && sb.gen == current {
				fmt.Println("[browser] idle, closing the shared browser")
				sb.shutdownLocked(false)
			}
		})
	}
}

// healthy reports whether gen is still the current browser and it responds.
func (sb *sharedBrowser) healthy(gen int) bool {
	sb.mu.Lock()
	defer sb.mu.Unlock()
	return gen == sb.gen && sb.browser != nil && sb.aliveLocked()
}

// aliveLocked checks the browser answers and the keeper tab still exists. It takes two missed
// checks in a row: one slow answer on a busy machine must not replace the browser under every
// worker. A crashed browser fails both at once.
func (sb *sharedBrowser) aliveLocked() bool {
	if sb.keeper == nil {
		return false
	}
	return browserAnswers(sb.browser, sb.keeper, browserHealthTimeout) ||
		browserAnswers(sb.browser, sb.keeper, browserHealthTimeout)
}

// browserAnswers is the liveness check: a crashed browser fails it at once, a hung one after timeout.
func browserAnswers(browser *rod.Browser, keeper *rod.Page, timeout time.Duration) bool {
	_, err := proto.TargetGetTargetInfo{TargetID: keeper.TargetID}.Call(browser.Timeout(timeout))
	return err == nil
}

// watch replaces browser gen when it stops answering twice in a row, so workers blocked in
// calls to a hung browser get an error (and retry in the new one) instead of waiting forever.
func (sb *sharedBrowser) watch(gen int, browser *rod.Browser, keeper *rod.Page, stop <-chan struct{}, interval, timeout time.Duration) {
	misses := 0
	for {
		select {
		case <-stop:
			return
		case <-time.After(interval):
		}
		if browserAnswers(browser, keeper, timeout) {
			misses = 0
			continue
		}
		if misses++; misses < 2 {
			continue
		}
		sb.mu.Lock()
		if sb.gen == gen && sb.browser != nil {
			fmt.Println("[browser] watchdog: shared browser stopped answering, killing it")
			sb.diedLocked()
		}
		sb.mu.Unlock()
		return
	}
}

// diedLocked records a dead (crashed or hung) browser for the crash-loop guard and kills it.
func (sb *sharedBrowser) diedLocked() {
	sb.deaths = append(sb.deaths, time.Now())
	sb.shutdownLocked(true)
}

// crashLoopLocked reports whether browsers died more often than crashLoopMaxRelaunches within crashLoopWindow.
func (sb *sharedBrowser) crashLoopLocked() bool {
	cutoff := time.Now().Add(-crashLoopWindow)
	recent := sb.deaths[:0]
	for _, d := range sb.deaths {
		if d.After(cutoff) {
			recent = append(recent, d)
		}
	}
	sb.deaths = recent
	return len(recent) > crashLoopMaxRelaunches
}

// shutdownLocked ends the browser; stop waits for its process to exit, so a replacement never
// starts on the profile while the old process still holds it. dead skips the graceful close
// and kills the browser's process group, leftover child processes included.
func (sb *sharedBrowser) shutdownLocked(dead bool) {
	stop := sb.stop
	sb.browser, sb.keeper, sb.stop = nil, nil, nil
	if sb.idleTimer != nil {
		sb.idleTimer.Stop()
		sb.idleTimer = nil
	}
	if sb.watchStop != nil {
		close(sb.watchStop)
		sb.watchStop = nil
	}
	if stop != nil {
		stop(dead)
	}
}

// openWindowPage opens a blank page in its own window of browser, with SVG capture installed.
// The caller must hold browser (acquire) until the page is closed with closeWindowPage.
func openWindowPage(browser *rod.Browser) (*rod.Page, error) {
	page, err := browser.Page(proto.TargetCreateTarget{URL: "", NewWindow: true})
	if err != nil {
		return nil, err
	}
	if err := installSVGCapture(page); err != nil {
		closeWindowPage(browser, page)
		return nil, err
	}
	return page, nil
}

// closeWindowPage closes a window without blocking other workers on a page that does not close.
func closeWindowPage(browser *rod.Browser, page *rod.Page) {
	if page == nil {
		return
	}
	_ = page.Timeout(windowCloseTimeout).Close()
	// rod's Close reports success when its timeout ends before the page is gone (a stuck
	// renderer never unloads it), so the window is always closed by id as well; for a window
	// already closed this is a no-op.
	abandonWindowPage(browser, page)
}

// abandonWindowPage closes a stuck window. Target.closeTarget is handled by the browser process,
// so it works while the page's renderer is stuck in a script, and the worker's calls blocked on
// that page return at once. Page.Close would first ask the stuck renderer to unload the page.
func abandonWindowPage(browser *rod.Browser, page *rod.Page) {
	if page == nil {
		return
	}
	_, _ = proto.TargetCloseTarget{TargetID: page.TargetID}.Call(browser.Timeout(windowCloseTimeout))
}

// windowResponds reports whether page still answers a trivial script within a few seconds.
func windowResponds(page *rod.Page) bool {
	if page == nil {
		return false
	}
	return pageAnswers(page)
}

// pageAnswers runs a trivial script in page, bounded by windowHealthTimeout. A page whose
// renderer is stuck in a script never answers it.
func pageAnswers(page *rod.Page) bool {
	_, err := proto.RuntimeEvaluate{Expression: "1"}.Call(page.Timeout(windowHealthTimeout))
	return err == nil
}

// windowWatch is the hang watchdog of one page of a worker's window.
type windowWatch struct {
	stop chan struct{}
	// hung is set once the watchdog closed the page for not answering
	hung atomic.Bool
}

// watchWindow starts page's hang watchdog. Workers blocked in a call to a stuck page (a click
// whose handler never returns, a script that never ends) never notice it themselves; closing
// the page frees them, while the shared browser and the other windows carry on.
func (sb *sharedBrowser) watchWindow(gen int, browser *rod.Browser, page *rod.Page) *windowWatch {
	ww := &windowWatch{stop: make(chan struct{})}
	interval, timeout := windowWatchInterval, windowHealthTimeout
	go func() {
		misses := 0
		for {
			select {
			case <-ww.stop:
				return
			case <-time.After(interval):
			}
			_, err := proto.RuntimeEvaluate{Expression: "1"}.Call(page.Timeout(timeout))
			if !errors.Is(err, context.DeadlineExceeded) {
				misses = 0 // answered, or failed fast (navigating, closed): not stuck
				continue
			}
			if misses++; misses < 2 {
				continue
			}
			select {
			case <-ww.stop:
				return // replaced or closed meanwhile
			default:
			}
			if !sb.healthy(gen) {
				// The whole browser is stuck or gone: its own watchdog replaces it
				misses = 0
				continue
			}
			fmt.Println("[browser] window watchdog: a window stopped answering, closing it")
			ww.hung.Store(true)
			abandonWindowPage(browser, page)
			return
		}
	}()
	return ww
}

func (ww *windowWatch) end() {
	if ww != nil {
		close(ww.stop)
	}
}

// browserWindow is one worker's own window in the shared browser. It holds the browser
// until Close, so Close must run on every path (defer it).
type browserWindow struct {
	sb      *sharedBrowser
	browser *rod.Browser
	gen     int
	held    bool
	Page    *rod.Page
	// watch is Page's hang watchdog
	watch *windowWatch
	// stuck counts stuck-window retries since an item last finished (see itemDone)
	stuck int
}

// openBrowserWindow opens a window (SVG capture installed) in the shared browser.
func openBrowserWindow() (*browserWindow, error) {
	return theSharedBrowser.openWindow()
}

// openWindow acquires the browser and opens a window in it. A browser that died before the
// window opened is replaced once.
func (sb *sharedBrowser) openWindow() (*browserWindow, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		browser, gen, err := sb.acquire()
		if err != nil {
			return nil, err
		}
		page, err := openWindowPage(browser)
		if err == nil {
			return &browserWindow{sb: sb, browser: browser, gen: gen, held: true, Page: page, watch: sb.watchWindow(gen, browser, page)}, nil
		}
		lastErr = err
		dead := !sb.healthy(gen)
		sb.release(gen, dead)
		if !dead {
			break
		}
		fmt.Println("[browser] browser died while opening a window, retrying in a new one:", err)
	}
	return nil, fmt.Errorf("cannot open browser window: %w", lastErr)
}

// Reopen replaces a stuck or failed window with a fresh blank one, leaving the other windows
// alone. When the browser itself died, it moves to the relaunched browser.
func (w *browserWindow) Reopen() error {
	if !w.held {
		return fmt.Errorf("browser window already closed")
	}
	old := w.Page
	w.Page = nil
	w.watch.end()
	w.watch = nil
	if w.sb.healthy(w.gen) {
		if windowResponds(old) {
			closeWindowPage(w.browser, old)
		} else {
			abandonWindowPage(w.browser, old)
		}
		page, err := openWindowPage(w.browser)
		if err == nil {
			w.Page = page
			w.watch = w.sb.watchWindow(w.gen, w.browser, page)
			return nil
		}
		if w.sb.healthy(w.gen) {
			return fmt.Errorf("cannot reopen browser window: %w", err)
		}
	}
	// A dead browser's windows end with it: no waiting on closing a window it can't answer for
	fmt.Println("[browser] shared browser died, moving the window to a new one")
	w.held = false
	w.sb.release(w.gen, true)
	nw, err := w.sb.openWindow()
	if err != nil {
		return err
	}
	nw.stuck = w.stuck
	*w = *nw
	return nil
}

// itemDone tells the window an item finished, so a later stuck window may be retried again.
func (w *browserWindow) itemDone() {
	w.stuck = 0
}

// runWithCrashRetry runs work in the window's page and runs it again when the browser or the
// window broke under it, whether work returned, failed or panicked:
//   - the browser died (crashed or hung): the window moves to the relaunched browser, up to
//     browserCrashRetries times;
//   - the window got stuck (its watchdog closed it, work reported errWindowStuck, or it failed
//     and the page no longer answers) or the page was not ready (errPageNotReady): the window
//     is replaced, up to windowStuckRetries times in a row (itemDone resets the count).
//
// work must skip items already done, so nothing is redone or re-uploaded. onFail runs before
// every rerun and before returning, to fail the item left "processing" (if any), so the rerun
// retries it and no item stays "processing" after an error, a panic or an early stop.
func (w *browserWindow) runWithCrashRetry(what string, work func(page *rod.Page) error, onFail func()) error {
	return w.runItems(what, work, onFail, nil)
}

// retryReason names why runItems retries in a fresh window: the window hung (its watchdog fired
// or the work reported errWindowStuck), the page answered but was not ready (errPageNotReady), or
// the window stopped answering after an error.
func retryReason(hung bool, err error) string {
	if !hung && errors.Is(err, errPageNotReady) {
		return "page not ready"
	}
	return "window stuck"
}

// runItems is runWithCrashRetry for a pass over a list of items. When the window is stuck (or
// the page not ready) again after its retry, giveUp fails the item being worked on and marks it
// for work to skip; the pass then runs again in a fresh window and goes on with the next item,
// instead of the worker dropping the rest of its list. giveUp returns false when there is no
// item left to give up (the pass fails before reaching one, or that item was given up already);
// runItems then returns the error.
func (w *browserWindow) runItems(what string, work func(page *rod.Page) error, onFail func(), giveUp func() bool) error {
	if onFail == nil {
		onFail = func() {}
	}
	defer onFail()
	crashes := 0
	for {
		err := callRecovering(func() error { return work(w.Page) })
		if w.sb.healthy(w.gen) {
			hung := w.watch != nil && w.watch.hung.Load() || errors.Is(err, errWindowStuck)
			stuck := hung || errors.Is(err, errPageNotReady) || err != nil && !windowResponds(w.Page)
			if !stuck {
				return err
			}
			reason := retryReason(hung, err)
			if w.stuck >= windowStuckRetries {
				onFail()
				if giveUp == nil || !giveUp() {
					fmt.Println("[browser]", reason, "again during", what, "- giving up:", err)
					if reason == "page not ready" {
						return fmt.Errorf("%s: the page was not ready: %v", what, err)
					}
					return fmt.Errorf("%s: the browser window got stuck: %v", what, err)
				}
				fmt.Println("[browser]", reason, "again during", what, "- failed that item, going on with the next in a new window:", err)
				w.stuck = 0
			} else {
				w.stuck++
				fmt.Println("[browser]", reason, "during", what, "- retrying it in a new window:", err)
			}
		} else {
			if crashes >= browserCrashRetries {
				fmt.Println("[browser] browser died again during", what, "- giving up:", err)
				return fmt.Errorf("%s: the browser died %d times: %v", what, crashes+1, err)
			}
			crashes++
			fmt.Println("[browser] browser died during", what, "- retrying it in a new browser:", err)
		}
		onFail()
		if rerr := w.Reopen(); rerr != nil {
			fmt.Println("[browser] cannot retry", what+":", rerr)
			return fmt.Errorf("%s: %w", what, rerr)
		}
	}
}

// callRecovering runs f, turning a panic (rod's Must* calls panic once the browser is gone) into an error.
func callRecovering(f func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return f()
}

// Close closes the window and releases the browser. Safe to call more than once.
func (w *browserWindow) Close() {
	if w == nil || !w.held {
		return
	}
	w.held = false
	// Released even when closing panics, so the browser can still idle out
	defer w.sb.release(w.gen, false)
	w.watch.end()
	w.watch = nil
	closeWindowPage(w.browser, w.Page)
}

// waitBrowserExit waits for the browser process to exit, killing it when it does not
// exit in time. false means it may still be running (and using its profile).
func waitBrowserExit(l *launcher.Launcher) bool {
	if l.PID() == 0 {
		return true // never started
	}
	// Cleanup waits for the process to exit and deletes the user data dir: unset the dir first
	l.Delete(flags.UserDataDir)
	exited := make(chan struct{})
	go func() {
		l.Cleanup()
		close(exited)
	}()
	select {
	case <-exited:
		return true
	case <-time.After(browserShutdownTimeout):
	}
	fmt.Println("[browser] browser did not exit, killing it")
	l.Kill()
	select {
	case <-exited:
		return true
	case <-time.After(browserShutdownTimeout):
		fmt.Println("[browser] browser still running after kill")
		return false
	}
}

// browserProfileDir keeps the shared browser's cache between launches.
func browserProfileDir() string {
	return filepath.Join(os.TempDir(), "owid-browser-profile")
}

// claimBrowserProfile returns the shared profile dir with an exclusive lock on it, or a fresh
// temp profile (lock nil) when another server process, or a browser of ours that never
// exited, holds the lock: two browsers must never share a profile.
func claimBrowserProfile() (string, *os.File, error) {
	dir := browserProfileDir()
	if err := os.MkdirAll(dir, 0o700); err == nil {
		if f, err := os.OpenFile(filepath.Join(dir, "owid-browser.lock"), os.O_CREATE|os.O_RDWR, 0o600); err == nil {
			if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil {
				// The lock proves no browser of ours runs here, so these are left by a killed one
				for _, name := range []string{"SingletonLock", "SingletonCookie", "SingletonSocket"} {
					_ = os.Remove(filepath.Join(dir, name))
				}
				removeKilledBrowserTempFiles(dir)
				return dir, f, nil
			}
			f.Close()
		}
	}
	fmt.Println("[browser] shared profile in use, using a temporary one")
	dir, err := os.MkdirTemp("", "owid-browser-profile-")
	return dir, nil, err
}

// removeKilledBrowserTempFiles removes the hidden temp files (.org.chromium.Chromium.*) that a
// killed browser leaves in the profile's Default folder, where it writes files atomically
// through a temp file. Only called with the profile lock held, so no browser is writing them.
func removeKilledBrowserTempFiles(profile string) {
	leftovers, _ := filepath.Glob(filepath.Join(profile, "Default", ".org.chromium.Chromium.*"))
	for _, f := range leftovers {
		_ = os.RemoveAll(f)
	}
}

var (
	retainedLocksMu sync.Mutex
	retainedLocks   []*os.File
)

// retainProfileLock keeps the profile lock of a browser that may still run reachable for the
// rest of the process: an unreferenced *os.File is closed by the GC, which would release the
// flock and let a second browser start on the same profile.
func retainProfileLock(f *os.File) {
	if f == nil {
		return
	}
	retainedLocksMu.Lock()
	retainedLocks = append(retainedLocks, f)
	retainedLocksMu.Unlock()
}

// closeOtherPages closes every tab but the keeper: the persistent profile brings back the
// previous session's tabs, which would otherwise pile up with each relaunch.
func closeOtherPages(browser *rod.Browser, keeper proto.TargetTargetID) error {
	targets, err := proto.TargetGetTargets{}.Call(browser)
	if err != nil {
		return err
	}
	for _, t := range targets.TargetInfos {
		if t.Type == proto.TargetTargetInfoTypePage && t.TargetID != keeper {
			// A tab that is already gone needs no closing
			_, _ = proto.TargetCloseTarget{TargetID: t.TargetID}.Call(browser)
		}
	}
	return nil
}

func launchSharedBrowserDefault() (*rod.Browser, *rod.Page, func(dead bool), error) {
	dir, lock, err := claimBrowserProfile()
	if err != nil {
		return nil, nil, nil, err
	}
	// The browser's own TMPDIR: a killed browser leaves its singleton socket dir
	// (org.chromium.Chromium.*) there, so each launch gets one, removed with the browser.
	tmp, err := os.MkdirTemp("", "owbt-")
	if err != nil {
		if lock != nil {
			lock.Close()
		} else {
			os.RemoveAll(dir)
		}
		return nil, nil, nil, err
	}
	// release frees the profile once the process has exited. A process that may still run
	// keeps the lock (or its temp dir), so no later browser starts on that profile.
	release := func(exited bool) {
		if !exited {
			retainProfileLock(lock)
			return
		}
		os.RemoveAll(tmp)
		if lock != nil {
			lock.Close()
		} else {
			os.RemoveAll(dir)
		}
	}

	var l *launcher.Launcher
	var browser *rod.Browser
	var keeper *rod.Page
	err = rod.Try(func() {
		l = sharedBrowserLauncher().
			UserDataDir(dir).
			Env(append(os.Environ(), "TMPDIR="+tmp)...).
			Set("disk-cache-size", "104857600") // 100 MB
		control := l.MustLaunch()
		browser = rod.New().ControlURL(control).MustConnect()
		keeper = browser.MustPage("about:blank")
		if err := closeOtherPages(browser, keeper.TargetID); err != nil {
			panic(err)
		}
	})
	if err != nil {
		if browser != nil {
			_ = browser.Timeout(browserShutdownTimeout).Close()
		}
		exited := true
		if l != nil {
			l.Kill()
			exited = waitBrowserExit(l)
		}
		release(exited)
		return nil, nil, nil, fmt.Errorf("launching browser: %w", err)
	}
	fmt.Println("[browser] shared browser started, pid", l.PID(), "profile", dir)
	stop := func(dead bool) {
		if dead {
			// No graceful close for a crashed or hung browser: kill its process group at once
			// (SIGKILL also ends a stopped process), so its children can't keep the profile
			l.Kill()
		} else {
			_ = browser.Timeout(browserShutdownTimeout).Close()
		}
		release(waitBrowserExit(l))
	}
	return browser, keeper, stop, nil
}
