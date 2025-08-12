package services

import (
	"github.com/go-rod/rod/lib/launcher"
	"github.com/wpmed-videowiki/OWIDImporter/env"
)

func GetLauncher() *launcher.Launcher {
	l := launcher.New()

	e := env.GetEnv()
	if e.OWID_ROD_BROWSER_DIR != "" {
		launcher.DefaultBrowserDir = e.OWID_ROD_BROWSER_DIR // "/workspace/.cache/rod/browser"
	}

	return l
}
