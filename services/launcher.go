package services

import (
	"fmt"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/wpmed-videowiki/OWIDImporter/env"
)

func GetLauncher() *launcher.Launcher {
	l := launcher.New()

	e := env.GetEnv()
	if e.OWID_ROD_BROWSER_DIR != "" {
		launcher.DefaultBrowserDir = e.OWID_ROD_BROWSER_DIR // "/workspace/.cache/rod/browser"
	}
	// l.Hosts = []launcher.Host{launcher.HostNPM, launcher.HostPlaywright}

	return l
}

func GetBrowser() (*launcher.Launcher, *rod.Browser) {
	e := env.GetEnv()
	if e.OWID_ROD_BROWSER_DIR != "" {
		launcher.DefaultBrowserDir = e.OWID_ROD_BROWSER_DIR // "/workspace/.cache/rod/browser"
	}
	// Download browser if not available
	b := launcher.NewBrowser()
	if e.OWID_ROD_BROWSER_DIR != "" {
		b.RootDir = e.OWID_ROD_BROWSER_DIR // "/workspace/.cache/rod/browser";
	}

	fmt.Println("Dir is", b.Dir(), b.RootDir)
	b.Hosts = []launcher.Host{launcher.HostNPM, launcher.HostPlaywright}
	_, err := b.Get()
	if err != nil {
		fmt.Println("Error getting browser", err)
	}

	l := launcher.New()

	control := l.Set("--no-sandbox").HeadlessNew(HEADLESS).MustLaunch()
	browser := rod.New().ControlURL(control).MustConnect()

	return l, browser
}
