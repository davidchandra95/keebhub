package httpapi

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v5"
)

type staticHandler struct {
	root       string
	filesystem fs.FS
}

func newStaticHandler(root string) staticHandler {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		absoluteRoot = root
	}
	return staticHandler{root: absoluteRoot, filesystem: os.DirFS(absoluteRoot)}
}

func (h staticHandler) serve(c *echo.Context) error {
	request := c.Request()
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return echo.ErrNotFound
	}

	cleanPath := path.Clean("/" + request.URL.Path)
	if isReservedPath(cleanPath) {
		return echo.ErrNotFound
	}

	relativePath := strings.TrimPrefix(cleanPath, "/")
	candidate := filepath.Join(h.root, filepath.FromSlash(relativePath))
	if h.insideRoot(candidate) {
		if info, err := fs.Stat(h.filesystem, filepath.ToSlash(relativePath)); err == nil && !info.IsDir() {
			if strings.HasPrefix(cleanPath, "/assets/") {
				c.Response().Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				c.Response().Header().Set("Cache-Control", "no-cache")
			}
			return c.FileFS(filepath.ToSlash(relativePath), h.filesystem)
		}
	}

	if path.Ext(cleanPath) != "" || !acceptsHTML(request.Header.Get("Accept")) {
		return echo.ErrNotFound
	}

	if info, err := fs.Stat(h.filesystem, "index.html"); err != nil || info.IsDir() {
		return echo.ErrNotFound
	}
	c.Response().Header().Set("Cache-Control", "no-cache")
	return c.FileFS("index.html", h.filesystem)
}

func (h staticHandler) insideRoot(candidate string) bool {
	relative, err := filepath.Rel(h.root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func isReservedPath(value string) bool {
	for _, prefix := range []string{"/api", "/auth", "/healthz", "/readyz"} {
		if value == prefix || strings.HasPrefix(value, prefix+"/") {
			return true
		}
	}
	return false
}

func acceptsHTML(accept string) bool {
	return accept == "" || strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*")
}
