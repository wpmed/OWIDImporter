package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
)

// The /iframe/info lookup shares one long-lived browser instead of launching one per request:
// a cold browser spends ~3s downloading OWID's scripts, a warm one with a filled cache ~0.9s.
// Import tasks keep launching their own browsers (GetBrowser) and never touch this one.

// lookupMaxConcurrent caps the lookups that may have a tab open at once; further lookups wait.
const lookupMaxConcurrent = 3

var (
	// lookupIdleTimeout closes the shared browser after this long without lookups (a var so tests can shorten it).
	lookupIdleTimeout = 10 * time.Minute
	// lookupHealthTimeout bounds the liveness check run before each lookup.
	lookupHealthTimeout = 3 * time.Second
	// lookupShutdownTimeout bounds closing the browser and waiting for its process to exit.
	lookupShutdownTimeout = 5 * time.Second
	// launchLookupBrowser starts the shared browser and returns it, its keeper tab and a stop
	// func that ends its process and releases its profile (a var so tests can substitute it).
	launchLookupBrowser = launchLookupBrowserDefault
	// runLookup performs one lookup in the shared browser, ended early when ctx ends (a var so tests can substitute it).
	runLookup = getIframeInfo
)

// lookupBrowser is the shared browser and its bookkeeping, guarded by mu.
type lookupBrowser struct {
	mu      sync.Mutex
	browser *rod.Browser
	// stop ends the browser process and releases its profile.
	stop func()
	// keeper is a blank tab that is never closed, so closing the last lookup tab can never
	// end the browser (a headful browser quits when its last window closes).
	keeper *rod.Page
	// gen identifies the current browser; a lookup holding an older gen knows it was replaced.
	gen       int
	inUse     int
	idleTimer *time.Timer
	slots     chan struct{}
}

var sharedLookupBrowser = newLookupBrowser()

func newLookupBrowser() *lookupBrowser {
	return &lookupBrowser{slots: make(chan struct{}, lookupMaxConcurrent)}
}

// LookupIframeInfo runs GetIframeInfo in the shared lookup browser. It waits for a free slot
// (giving up when ctx ends) and retries once in a fresh browser when the browser died mid-lookup.
func LookupIframeInfo(ctx context.Context, input string) (*IframeInfo, error) {
	return sharedLookupBrowser.lookup(ctx, input)
}

func (lb *lookupBrowser) lookup(ctx context.Context, input string) (*IframeInfo, error) {
	select {
	case lb.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-lb.slots }()

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		info, died, err := lb.attempt(ctx, input)
		if !died {
			return info, err
		}
		fmt.Println("[lookup browser] browser died during lookup, retrying in a new one:", input, err)
		lastErr = err
	}
	return nil, lastErr
}

// attempt runs one lookup in the shared browser; died reports that the browser died during it.
func (lb *lookupBrowser) attempt(ctx context.Context, input string) (*IframeInfo, bool, error) {
	browser, gen, err := lb.acquire()
	if err != nil {
		return nil, false, err
	}
	died := false
	// Deferred so a panicking lookup still releases the browser and lets it idle out
	defer func() { lb.release(gen, died) }()
	info, err := runLookup(ctx, browser, input)
	// A lookup the client gave up on, or one that failed in a healthy browser (the chart's
	// fault), is reported as is: no retry and no replacement.
	if err == nil || ctx.Err() != nil || lb.healthy(gen) {
		return info, false, err
	}
	died = true
	return nil, true, err
}

// acquire returns the shared browser, launching it (or replacing a dead one) when needed.
func (lb *lookupBrowser) acquire() (*rod.Browser, int, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	if lb.idleTimer != nil {
		lb.idleTimer.Stop()
		lb.idleTimer = nil
	}
	if lb.browser != nil && !lb.aliveLocked() {
		fmt.Println("[lookup browser] browser not responding, replacing it")
		lb.shutdownLocked()
	}
	if lb.browser == nil {
		browser, keeper, stop, err := launchLookupBrowser()
		if err != nil {
			return nil, 0, fmt.Errorf("cannot start lookup browser: %w", err)
		}
		lb.browser, lb.keeper, lb.stop = browser, keeper, stop
		lb.gen++
	}
	lb.inUse++
	return lb.browser, lb.gen, nil
}

// release ends a lookup. dead replaces the browser (when it is still the current one);
// the last lookup out starts the idle timer.
func (lb *lookupBrowser) release(gen int, dead bool) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.inUse--
	if dead && gen == lb.gen {
		lb.shutdownLocked()
	}
	if lb.inUse == 0 && lb.browser != nil {
		current := lb.gen
		lb.idleTimer = time.AfterFunc(lookupIdleTimeout, func() {
			lb.mu.Lock()
			defer lb.mu.Unlock()
			if lb.inUse == 0 && lb.browser != nil && lb.gen == current {
				fmt.Println("[lookup browser] idle, closing it")
				lb.shutdownLocked()
			}
		})
	}
}

// healthy reports whether gen is still the current browser and it responds.
func (lb *lookupBrowser) healthy(gen int) bool {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	return gen == lb.gen && lb.browser != nil && lb.aliveLocked()
}

// aliveLocked checks the browser answers and the keeper tab still exists, in one call.
func (lb *lookupBrowser) aliveLocked() bool {
	if lb.keeper == nil {
		return false
	}
	_, err := proto.TargetGetTargetInfo{TargetID: lb.keeper.TargetID}.Call(lb.browser.Timeout(lookupHealthTimeout))
	return err == nil
}

// shutdownLocked ends the browser; stop waits for its process to exit, so a replacement never
// starts on the profile while the old process still holds it.
func (lb *lookupBrowser) shutdownLocked() {
	stop := lb.stop
	lb.browser, lb.keeper, lb.stop = nil, nil, nil
	if lb.idleTimer != nil {
		lb.idleTimer.Stop()
		lb.idleTimer = nil
	}
	if stop != nil {
		stop()
	}
}

// waitLookupBrowserExit waits for the browser process to exit, killing it when it does not
// exit in time. false means it may still be running (and using its profile).
func waitLookupBrowserExit(l *launcher.Launcher) bool {
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
	case <-time.After(lookupShutdownTimeout):
	}
	fmt.Println("[lookup browser] browser did not exit, killing it")
	l.Kill()
	select {
	case <-exited:
		return true
	case <-time.After(lookupShutdownTimeout):
		fmt.Println("[lookup browser] browser still running after kill")
		return false
	}
}

// lookupProfileDir keeps the lookup browser's cache between launches.
func lookupProfileDir() string {
	return filepath.Join(os.TempDir(), "owid-iframe-lookup-profile")
}

// claimLookupProfile returns the shared profile dir with an exclusive lock on it, or a fresh
// temp profile (lock nil) when another server process, or a browser of ours that never
// exited, holds the lock: two browsers must never share a profile.
func claimLookupProfile() (string, *os.File, error) {
	dir := lookupProfileDir()
	if err := os.MkdirAll(dir, 0o700); err == nil {
		if f, err := os.OpenFile(filepath.Join(dir, "owid-lookup.lock"), os.O_CREATE|os.O_RDWR, 0o600); err == nil {
			if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) == nil {
				// The lock proves no browser of ours runs here, so these are left by a killed one
				for _, name := range []string{"SingletonLock", "SingletonCookie", "SingletonSocket"} {
					_ = os.Remove(filepath.Join(dir, name))
				}
				return dir, f, nil
			}
			f.Close()
		}
	}
	fmt.Println("[lookup browser] shared profile in use, using a temporary one")
	dir, err := os.MkdirTemp("", "owid-iframe-lookup-profile-")
	return dir, nil, err
}

var (
	retainedLocksMu sync.Mutex
	retainedLocks   []*os.File
)

// retainLookupLock keeps the profile lock of a browser that may still run reachable for the
// rest of the process: an unreferenced *os.File is closed by the GC, which would release the
// flock and let a second browser start on the same profile.
func retainLookupLock(f *os.File) {
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

func launchLookupBrowserDefault() (*rod.Browser, *rod.Page, func(), error) {
	dir, lock, err := claimLookupProfile()
	if err != nil {
		return nil, nil, nil, err
	}
	// release frees the profile once the process has exited. A process that may still run
	// keeps the lock (or its temp dir), so no later browser starts on that profile.
	release := func(exited bool) {
		if !exited {
			retainLookupLock(lock)
			return
		}
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
		l = newBrowserLauncher().
			UserDataDir(dir).
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
			_ = browser.Timeout(lookupShutdownTimeout).Close()
		}
		exited := true
		if l != nil {
			l.Kill()
			exited = waitLookupBrowserExit(l)
		}
		release(exited)
		return nil, nil, nil, fmt.Errorf("launching lookup browser: %w", err)
	}
	fmt.Println("[lookup browser] started, profile", dir)
	stop := func() {
		_ = browser.Timeout(lookupShutdownTimeout).Close()
		release(waitLookupBrowserExit(l))
	}
	return browser, keeper, stop, nil
}
