package services

import (
	"context"
	"fmt"
)

// The /iframe/info lookup runs in the shared browser (see shared_browser.go), so it starts warm:
// a cold browser spends ~3s downloading OWID's scripts, a warm one with a filled cache ~0.9s.

// lookupMaxConcurrent caps the lookups that may have a window open at once; further lookups wait.
const lookupMaxConcurrent = 3

// runLookup performs one lookup in the shared browser, ended early when ctx ends (a var so tests can substitute it).
var runLookup = getIframeInfo

// lookupBrowser caps the lookups running in the shared browser.
type lookupBrowser struct {
	sb    *sharedBrowser
	slots chan struct{}
}

var sharedLookupBrowser = newLookupBrowser(theSharedBrowser)

func newLookupBrowser(sb *sharedBrowser) *lookupBrowser {
	return &lookupBrowser{sb: sb, slots: make(chan struct{}, lookupMaxConcurrent)}
}

// LookupIframeInfo runs GetIframeInfo in the shared browser. It waits for a free slot
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
		fmt.Println("[lookup] browser died during lookup, retrying in a new one:", input, err)
		lastErr = err
	}
	return nil, lastErr
}

// attempt runs one lookup in the shared browser; died reports that the browser died during it.
func (lb *lookupBrowser) attempt(ctx context.Context, input string) (*IframeInfo, bool, error) {
	browser, gen, err := lb.sb.acquire()
	if err != nil {
		return nil, false, err
	}
	died := false
	// Deferred so a panicking lookup still releases the browser and lets it idle out
	defer func() { lb.sb.release(gen, died) }()
	info, err := runLookup(ctx, browser, input)
	// A lookup the client gave up on, or one that failed in a healthy browser (the chart's
	// fault), is reported as is: no retry and no replacement.
	if err == nil || ctx.Err() != nil || lb.sb.healthy(gen) {
		return info, false, err
	}
	died = true
	return nil, true, err
}
