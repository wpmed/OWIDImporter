package services

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/wpmed-videowiki/OWIDImporter/models"
)

// fakeWiki is a minimal MediaWiki API: respond picks the JSON reply for each
// request; every request's parameters (URL query plus multipart form) are recorded.
type fakeWiki struct {
	mu      sync.Mutex
	calls   []map[string]string
	respond func(p map[string]string) any
}

func newFakeWiki(t *testing.T, respond func(p map[string]string) any) *fakeWiki {
	t.Helper()
	fw := &fakeWiki{respond: respond}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := map[string]string{}
		for k, v := range r.URL.Query() {
			p[k] = v[0]
		}
		if r.Method == http.MethodPost {
			if err := r.ParseMultipartForm(10 << 20); err == nil {
				for k, v := range r.MultipartForm.Value {
					p[k] = v[0]
				}
			}
		}
		fw.mu.Lock()
		fw.calls = append(fw.calls, p)
		fw.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fw.respond(p))
	}))
	t.Cleanup(srv.Close)
	for k, v := range map[string]string{
		"OWID_UA": "test", "OWID_MW_API": srv.URL, "OWID_ENV": "test", "OWID_ENCRYPTION_KEY": "k",
		"OWID_OAUTH_TOKEN": "t", "OWID_OAUTH_SECRET": "s", "OWID_OAUTH_INITIATE": "http://x",
		"OWID_OAUTH_AUTH": "http://x", "OWID_OAUTH_TOKEN_URL": "http://x",
	} {
		t.Setenv(k, v)
	}
	return fw
}

func (fw *fakeWiki) callsWith(key, value string) []map[string]string {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	var out []map[string]string
	for _, c := range fw.calls {
		if c[key] == value {
			out = append(out, c)
		}
	}
	return out
}

func TestSearchPageWithPrefixListsNonRedirectsByPrefix(t *testing.T) {
	fw := newFakeWiki(t, func(p map[string]string) any {
		if p["apcontinue"] == "" {
			return map[string]any{
				"continue": map[string]string{"apcontinue": "Banana_production,_1961_to_2023,_BRA.svg", "continue": "-||"},
				"query":    map[string]any{"allpages": []map[string]any{{"ns": 6, "title": "File:Banana production, 1961 to 2023, ARE.svg"}}},
			}
		}
		return map[string]any{"query": map[string]any{"allpages": []map[string]any{{"ns": 6, "title": "File:Banana production, 1961 to 2023, BRA.svg"}}}}
	})

	titles, err := SearchPageWithPrefix(&models.User{}, "Banana production, 1961 to ")
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 2 || titles[0] != "File:Banana production, 1961 to 2023, ARE.svg" || titles[1] != "File:Banana production, 1961 to 2023, BRA.svg" {
		t.Fatalf("titles = %v", titles)
	}
	calls := fw.callsWith("list", "allpages")
	if len(calls) != 2 {
		t.Fatalf("want 2 allpages requests (with continuation), got %d", len(calls))
	}
	first := calls[0]
	for k, want := range map[string]string{"apnamespace": "6", "apprefix": "Banana production, 1961 to ", "apfilterredir": "nonredirects"} {
		if first[k] != want {
			t.Errorf("%s = %q, want %q", k, first[k], want)
		}
	}
	if first["generator"] != "" || first["redirects"] != "" {
		t.Errorf("must not use full-text search or follow redirects: %v", first)
	}
}

func TestMovePageDryRunMakesNoRequest(t *testing.T) {
	fw := newFakeWiki(t, func(p map[string]string) any { return map[string]any{} })
	t.Setenv("OWID_MOVE_DRY_RUN", "1")

	err := MovePage(&models.User{}, "token", "File:A, 1961 to 2023, BRA.svg", "File:A, BRA.svg")
	if !errors.Is(err, errMoveDryRun) {
		t.Fatalf("err = %v, want errMoveDryRun", err)
	}
	if len(fw.calls) != 0 {
		t.Fatalf("dry run sent %d requests", len(fw.calls))
	}
}

// writeUploadDir writes the single SVG uploadMapFile expects in its download directory.
func writeUploadDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><text>World 1961</text><path d="M0 0 L1 1"/></svg>`
	if err := os.WriteFile(filepath.Join(dir, "image.svg"), []byte(svg), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func uploadTestData() (ReplaceVarsData, StartData) {
	return ReplaceVarsData{Url: "https://ourworldindata.org/grapher/banana-production", FileName: "Banana production", Region: "World", Year: "1961", Comment: "Importing"},
		StartData{Url: "https://ourworldindata.org/grapher/banana-production", FileName: "$NAME, $REGION, $YEAR.svg", Description: "World map $YEAR"}
}

func TestUploadMapFileReplacesRedirect(t *testing.T) {
	fw := newFakeWiki(t, func(p map[string]string) any {
		switch {
		case p["prop"] == "imageinfo|info":
			// Commons follows the file redirect: imageinfo is the target's (the ARE chart)
			return map[string]any{"query": map[string]any{"pages": []map[string]any{{
				"ns": 6, "title": "File:Banana production, World, 1961.svg", "redirect": true,
				"imageinfo": []map[string]string{{"sha1": "aaaa", "url": "https://upload.example/Banana_production,_ARE.svg"}},
			}}}}
		case p["list"] == "allimages":
			return map[string]any{"query": map[string]any{"allimages": []any{}}}
		case p["action"] == "upload":
			return map[string]any{"upload": map[string]any{"result": "Success"}}
		case p["prop"] == "revisions":
			return map[string]any{"query": map[string]any{"pages": []map[string]any{{
				"ns": 6, "title": "File:Banana production, World, 1961.svg",
				"revisions": []map[string]any{{"slots": map[string]any{"main": map[string]string{"content": "#REDIRECT [[File:Banana production, ARE.svg]]"}}}},
			}}}}
		case p["action"] == "edit":
			return map[string]any{"edit": map[string]string{"result": "Success"}}
		}
		t.Errorf("unexpected request %v", p)
		return map[string]any{}
	})
	replaceData, data := uploadTestData()

	name, status, err := uploadMapFile(&models.User{}, "token", replaceData, writeUploadDir(t), data)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Banana production, World, 1961.svg" || status != "uploaded" {
		t.Fatalf("got %q %q", name, status)
	}
	uploads := fw.callsWith("action", "upload")
	if len(uploads) != 1 || uploads[0]["filename"] != "Banana production, World, 1961.svg" {
		t.Fatalf("want one upload to the redirect name, got %v", uploads)
	}
	edits := fw.callsWith("action", "edit")
	if len(edits) != 1 || edits[0]["title"] != "File:Banana production, World, 1961.svg" || edits[0]["text"] != "World map 1961" {
		t.Fatalf("want the redirect replaced by the description, got %v", edits)
	}
}

func TestUploadMapFileNewFileDoesNotEditDescription(t *testing.T) {
	fw := newFakeWiki(t, func(p map[string]string) any {
		switch {
		case p["prop"] == "imageinfo|info":
			return map[string]any{"query": map[string]any{"pages": []map[string]any{{"ns": 6, "title": "File:Banana production, World, 1961.svg", "missing": true}}}}
		case p["list"] == "allimages":
			return map[string]any{"query": map[string]any{"allimages": []any{}}}
		case p["action"] == "upload":
			return map[string]any{"upload": map[string]any{"result": "Success"}}
		}
		t.Errorf("unexpected request %v", p)
		return map[string]any{}
	})
	replaceData, data := uploadTestData()

	_, status, err := uploadMapFile(&models.User{}, "token", replaceData, writeUploadDir(t), data)
	if err != nil || status != "uploaded" {
		t.Fatalf("status %q err %v", status, err)
	}
	if n := len(fw.callsWith("action", "edit")); n != 0 {
		t.Fatalf("new file upload made %d description edits", n)
	}
}
