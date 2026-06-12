package services

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/wpmed-videowiki/OWIDImporter/env"
)

var (
	bravePath        string
	braveInstallOnce sync.Once
)

func GetLauncher() *launcher.Launcher {
	l := launcher.New()

	e := env.GetEnv()
	if e.OWID_ROD_BROWSER_DIR != "" {
		launcher.DefaultBrowserDir = e.OWID_ROD_BROWSER_DIR
	}

	return l
}

func findBrave() string {
	if path, err := exec.LookPath("brave-browser"); err == nil {
		return path
	}

	if path, err := exec.LookPath("brave"); err == nil {
		return path
	}

	return ""
}

func findOrInstallBrave() string {
	braveInstallOnce.Do(func() {
		// Check if Brave is already installed
		if path := findBrave(); path != "" {
			fmt.Printf("[browser] Found Brave at %s\n", path)
			bravePath = path
			return
		}

		fmt.Println("[browser] Brave not found. Attempting installation...")

		cmd := exec.Command(
			"bash",
			"-c",
			"curl -fsS https://dl.brave.com/install.sh | sh",
		)

		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			fmt.Printf("[browser] Failed to install Brave: %v\n", err)
			fmt.Println("[browser] Will fall back to Rod-managed Chromium.")
			return
		}

		// Check again after install
		if path := findBrave(); path != "" {
			fmt.Printf("[browser] Successfully installed Brave at %s\n", path)
			bravePath = path
			return
		}

		fmt.Println("[browser] Brave install script completed, but no Brave executable was found in PATH.")
		fmt.Println("[browser] Will fall back to Rod-managed Chromium.")
	})

	return bravePath
}

func GetBrowser() (*launcher.Launcher, *rod.Browser) {
	e := env.GetEnv()

	if e.OWID_ROD_BROWSER_DIR != "" {
		launcher.DefaultBrowserDir = e.OWID_ROD_BROWSER_DIR
	}

	var binPath string

	// Try Brave first
	binPath = findOrInstallBrave()

	// Fallback to Rod-managed Chromium
	if binPath == "" {
		fmt.Println("[browser] Using Rod-managed Chromium fallback.")

		b := launcher.NewBrowser()

		if e.OWID_ROD_BROWSER_DIR != "" {
			b.RootDir = e.OWID_ROD_BROWSER_DIR
		}

		b.Hosts = []launcher.Host{
			launcher.HostNPM,
			launcher.HostPlaywright,
		}

		var err error
		binPath, err = b.Get()
		if err != nil {
			fmt.Printf("[browser] Failed to download Chromium: %v\n", err)
			panic(err)
		}

		fmt.Printf("[browser] Chromium available at %s\n", binPath)
	}

	fmt.Printf("[browser] Launching browser: %s\n", binPath)

	l := launcher.New().
		Bin(binPath).
		Set("--no-sandbox").
		HeadlessNew(HEADLESS)

	control := l.MustLaunch()

	browser := rod.New().
		ControlURL(control).
		MustConnect()

	return l, browser
}
