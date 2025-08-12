package env

import (
	"fmt"
	"os"
	"strconv"
)

type EnvVariables struct {
	OWID_UA              string
	OWID_OAUTH_TOKEN     string
	OWID_OAUTH_SECRET    string
	OWID_OAUTH_INITIATE  string
	OWID_OAUTH_AUTH      string
	OWID_OAUTH_TOKEN_URL string
	OWID_MW_API          string
	OWID_DEBUG           bool
	OWID_ENV             string
	OWID_ENCRYPTION_KEY  string
	OWID_ROD_BROWSER_DIR string
}

func GetEnv() EnvVariables {
	userAgent := os.Getenv("OWID_UA")
	if userAgent == "" {
		panic("OWID_UA environment variable is required")
	}
	OWID_DEBUG, err := strconv.ParseBool(os.Getenv("OWID_DEBUG"))
	if err != nil {
		OWID_DEBUG = false
	}

	oauthToken := os.Getenv("OWID_OAUTH_TOKEN")
	if oauthToken == "" {
		panic("OWID_OAUTH_TOKEN environment variable is required")
	}

	oauthSecret := os.Getenv("OWID_OAUTH_SECRET")
	if oauthSecret == "" {
		panic("OWID_OAUTH_SECRET environment variable is required")
	}

	oauthInitiate := os.Getenv("OWID_OAUTH_INITIATE")
	if oauthInitiate == "" {
		panic("OWID_OAUTH_INITIATE environment variable is required")
	}

	oauthAuth := os.Getenv("OWID_OAUTH_AUTH")
	if oauthAuth == "" {
		panic("OWID_OAUTH_AUTH environment variable is required")
	}

	oauthTokenUrl := os.Getenv("OWID_OAUTH_TOKEN_URL")
	if oauthTokenUrl == "" {
		panic("OWID_OAUTH_TOKEN_URL environment variable is required")
	}

	mwApi := os.Getenv("OWID_MW_API")
	if mwApi == "" {
		panic("OWID_MW_API environment variable is required")
	}

	owidEnv := os.Getenv("OWID_ENV")
	if owidEnv == "" {
		panic("OWID_ENV environment variable is required")
	}

	owidEncKey := os.Getenv("OWID_ENCRYPTION_KEY")
	if owidEnv == "" {
		panic("OWID_ENCRYPTION_KEY environment variable is required")
	}

	rodBrowserDir := os.Getenv("OWID_ROD_BROWSER_DIR")
	if rodBrowserDir == "" {
		fmt.Println("Warning: OWID_ROD_BROWSER_DIR environment variable is not set. Using environment default")
	}

	return EnvVariables{
		OWID_UA:              userAgent,
		OWID_OAUTH_TOKEN:     oauthToken,
		OWID_OAUTH_SECRET:    oauthSecret,
		OWID_OAUTH_INITIATE:  oauthInitiate,
		OWID_OAUTH_AUTH:      oauthAuth,
		OWID_OAUTH_TOKEN_URL: oauthTokenUrl,
		OWID_MW_API:          mwApi,
		OWID_DEBUG:           OWID_DEBUG,
		OWID_ENV:             owidEnv,
		OWID_ENCRYPTION_KEY:  owidEncKey,
		OWID_ROD_BROWSER_DIR: rodBrowserDir,
	}
}
