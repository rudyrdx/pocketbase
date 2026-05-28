package apis

import (
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

// SPAStatic returns a handler tuned for serving a Next.js-style static export
// alongside the standard PocketBase routes. It's a superset of Static() with
// the SPA fallback semantics our frontends rely on:
//
//   - direct file requests are served as-is (Strategy 1)
//   - "pretty" URLs without an extension fall back to <path>.html (Strategy 2)
//   - directory routes resolve to <path>/index.html with a canonical
//     trailing-slash redirect (Strategy 3)
//   - missing static assets (.css/.js/.png/...) return 404 instead of
//     swallowing them into index.html (Strategy 4)
//   - everything else falls through to index.html so client-side routing
//     (e.g. /dashboard, /login) can take over (Strategy 5)
//
// This was a local patch on v0.29.2 of pocketbase; preserving it here keeps
// the static-export behaviour identical across PB upgrades.
//
// NB! Expects the route to have a "{path...}" wildcard parameter.
//
// Example:
//
//	router.GET("/{path...}", apis.SPAStatic(fsys))
func SPAStatic(fsys fs.FS) func(*core.RequestEvent) error {
	if fsys == nil {
		panic("SPAStatic: the provided fs.FS argument is nil")
	}

	return func(e *core.RequestEvent) error {
		// disable the activity logger to avoid flooding with messages
		if e.Get(requestEventKeySkipSuccessActivityLog) == nil {
			e.Set(requestEventKeySkipSuccessActivityLog, true)
		}

		path := e.Request.PathValue(StaticWildcardParam)
		if path == "" {
			path = e.Request.URL.Path
		}

		filename := filepath.ToSlash(filepath.Clean(strings.TrimPrefix(path, "/")))

		// Handle root path
		if filename == "" {
			return e.FileFS(fsys, router.IndexPage)
		}

		// Security check for directory traversal
		if len(filename) > 2 && filename[0] == '.' && filename[1] == '.' && (filename[2] == '/' || filename[2] == '\\') {
			return e.FileFS(fsys, router.IndexPage)
		}

		// Strategy 1: Try exact file match first
		if fi, err := fs.Stat(fsys, filename); err == nil {
			if fi.IsDir() {
				// For directories, redirect to trailing slash and serve index.html
				if !strings.HasSuffix(e.Request.URL.Path, "/") {
					return e.Redirect(http.StatusMovedPermanently, safeRedirectPath(e.Request.URL.Path+"/"))
				}
				// Try directory/index.html
				indexPath := filepath.Join(filename, router.IndexPage)
				if _, indexErr := fs.Stat(fsys, indexPath); indexErr == nil {
					return e.FileFS(fsys, indexPath)
				}
			} else {
				// Handle clean URLs by redirecting /page.html to /page
				urlPath := e.Request.URL.Path
				if strings.HasSuffix(urlPath, "/") {
					urlPath = strings.TrimRight(urlPath, "/")
					if len(urlPath) > 0 {
						return e.Redirect(http.StatusMovedPermanently, safeRedirectPath(urlPath))
					}
				} else if stripped, ok := strings.CutSuffix(urlPath, router.IndexPage); ok {
					return e.Redirect(http.StatusMovedPermanently, safeRedirectPath(stripped))
				}

				return e.FileFS(fsys, filename)
			}
		}

		// Strategy 2: Try with .html extension for pretty routes
		htmlFilename := filename + ".html"
		if fi, err := fs.Stat(fsys, htmlFilename); err == nil && !fi.IsDir() {
			return e.FileFS(fsys, htmlFilename)
		}

		// Strategy 3: Try as directory with index.html (for nested routes)
		indexInDir := filepath.Join(filename, router.IndexPage)
		if fi, err := fs.Stat(fsys, indexInDir); err == nil && !fi.IsDir() {
			// Redirect to canonical URL with trailing slash
			if !strings.HasSuffix(e.Request.URL.Path, "/") {
				return e.Redirect(http.StatusMovedPermanently, safeRedirectPath(e.Request.URL.Path+"/"))
			}
			return e.FileFS(fsys, indexInDir)
		}

		// Strategy 4: Check if this looks like a static asset request
		// If so, return 404 instead of falling back to index.html
		ext := filepath.Ext(filename)
		staticExtensions := map[string]bool{
			".css": true, ".js": true, ".png": true, ".jpg": true, ".jpeg": true,
			".gif": true, ".svg": true, ".ico": true, ".woff": true, ".woff2": true,
			".ttf": true, ".eot": true, ".map": true, ".json": true, ".xml": true,
			".txt": true, ".pdf": true, ".zip": true,
		}

		if staticExtensions[ext] {
			return router.ErrFileNotFound
		}

		// Strategy 5: Fall back to index.html for SPA client-side routing
		// This handles routes like /login, /dashboard, etc.
		return e.FileFS(fsys, router.IndexPage)
	}
}
