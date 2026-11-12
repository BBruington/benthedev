package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"github.com/BBruington/colony-planner/internal/auth"
)

// The Foundry module, served by Colony Planner itself (design §12.4): Foundry's "Install From
// Manifest" fetches module.json, whose manifest and download point back at PUBLIC_URL. The version is
// the one the module build wrote into dist/module.json (foundry/package.json's), since the server
// binary carries no version of its own.

const (
	foundryManifestPath = "/foundry/module.json"
	foundryZipPath      = "/foundry/colony-bridge.zip"
	foundryManifestName = "module.json"
	missingModuleNote   = "This server has no Foundry module embedded. Run mise run build to embed it."
)

// servedModule is the module as this server serves it, built once at startup. The zip's own
// module.json is the served manifest too: the installed module finds Colony Planner from its manifest
// field, and Foundry's update check compares the two.
type servedModule struct {
	manifest []byte
	archive  []byte
}

// buildServedModule reads the module build; without one both routes answer 404, and a zip it can't
// rewrite refuses startup.
func buildServedModule(distribution fs.FS, publicURL string) (servedModule, error) {
	manifest, built := builtManifest(distribution, strings.TrimSuffix(publicURL, "/"))
	if !built {
		return servedModule{}, nil
	}
	archive, err := fs.ReadFile(distribution, "dist/colony-bridge.zip")
	if err != nil {
		return servedModule{manifest: manifest}, nil
	}
	rewritten, err := withManifest(archive, manifest)
	if err != nil {
		return servedModule{}, fmt.Errorf("app: put the served manifest into the Foundry module zip: %w", err)
	}
	return servedModule{manifest: manifest, archive: rewritten}, nil
}

func builtManifest(distribution fs.FS, publicURL string) ([]byte, bool) {
	built, err := fs.ReadFile(distribution, "dist/"+foundryManifestName)
	if err != nil {
		return nil, false
	}
	var manifest map[string]any
	if err := json.Unmarshal(built, &manifest); err != nil {
		return nil, false
	}
	manifest["manifest"] = publicURL + foundryManifestPath
	manifest["download"] = publicURL + foundryZipPath
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, false
	}
	return encoded, true
}

// withManifest replaces the archive's module.json with manifest and copies every other entry as it is.
func withManifest(archive []byte, manifest []byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, err
	}
	var rewritten bytes.Buffer
	writer := zip.NewWriter(&rewritten)
	for _, file := range reader.File {
		if file.Name != foundryManifestName {
			if err := writer.Copy(file); err != nil {
				return nil, err
			}
			continue
		}
		header := file.FileHeader
		header.Method = zip.Store
		entry, err := writer.CreateHeader(&header)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write(manifest); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return rewritten.Bytes(), nil
}

// foundryManifest answers GET /foundry/module.json (public).
func foundryManifest(module servedModule) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if module.manifest == nil {
			auth.WriteProblem(writer, http.StatusNotFound, missingModuleNote)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Cache-Control", "no-cache")
		_, _ = writer.Write(module.manifest)
	})
}

// foundryZip answers GET /foundry/colony-bridge.zip (public); the module holds no secrets.
func foundryZip(module servedModule) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if module.archive == nil {
			auth.WriteProblem(writer, http.StatusNotFound, missingModuleNote)
			return
		}
		writer.Header().Set("Content-Type", "application/zip")
		writer.Header().Set("Cache-Control", "no-cache")
		_, _ = writer.Write(module.archive)
	})
}
