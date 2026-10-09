package middleware

import (
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// Decoy serves the static site in dir to every request the panel does not
// answer itself, so a scanner sees an ordinary web site rather than a panel's
// bare 404. It only reads files: GET and HEAD, no directory listings, nothing
// whose path has a part starting with a dot, and nothing a symlink leads out
// of dir. A missing page gets the site's own 404.html, or an empty 404.
func Decoy(dir string) gin.HandlerFunc {
	return func(c *gin.Context) {
		serveDecoy(c, dir)
	}
}

func serveDecoy(c *gin.Context, dir string) {
	root, err := filepath.EvalSymlinks(filepath.Clean(dir))
	if err != nil || dir == "" {
		c.String(http.StatusNotFound, "")
		return
	}
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		decoyNotFound(c, root)
		return
	}
	urlPath := c.Request.URL.Path
	clean := path.Clean("/" + urlPath)
	for _, part := range strings.Split(clean, "/") {
		if strings.HasPrefix(part, ".") {
			decoyNotFound(c, root)
			return
		}
	}
	file, ok := insideRoot(root, clean)
	if !ok {
		decoyNotFound(c, root)
		return
	}
	st, err := os.Stat(file)
	if err != nil {
		decoyNotFound(c, root)
		return
	}
	if st.IsDir() {
		index, ok := insideRoot(root, path.Join(clean, "index.html"))
		if !ok {
			decoyNotFound(c, root)
			return
		}
		ist, err := os.Stat(index)
		if err != nil || !ist.Mode().IsRegular() {
			decoyNotFound(c, root)
			return
		}
		// Like any web server: a folder is addressed with its slash, so the
		// page's relative links resolve inside it.
		if !strings.HasSuffix(urlPath, "/") {
			target := urlPath + "/"
			if q := c.Request.URL.RawQuery; q != "" {
				target += "?" + q
			}
			c.Redirect(http.StatusMovedPermanently, target)
			return
		}
		file, st = index, ist
	}
	if !st.Mode().IsRegular() {
		decoyNotFound(c, root)
		return
	}
	serveDecoyFile(c, file, http.StatusOK)
}

// insideRoot resolves a clean URL path under root, following symlinks, and
// refuses what ends up outside it.
func insideRoot(root, clean string) (string, bool) {
	full := filepath.Join(root, filepath.FromSlash(clean))
	resolved, err := filepath.EvalSymlinks(full)
	if err != nil {
		// A path that does not exist resolves nowhere; the caller's stat
		// answers 404 for it.
		if os.IsNotExist(err) {
			return full, strings.HasPrefix(full, root)
		}
		return "", false
	}
	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return "", false
	}
	return resolved, true
}

func decoyNotFound(c *gin.Context, root string) {
	if page, ok := insideRoot(root, "/404.html"); ok {
		if st, err := os.Stat(page); err == nil && st.Mode().IsRegular() {
			serveDecoyFile(c, page, http.StatusNotFound)
			return
		}
	}
	c.String(http.StatusNotFound, "")
}

func serveDecoyFile(c *gin.Context, file string, status int) {
	f, err := os.Open(file)
	if err != nil {
		c.String(http.StatusNotFound, "")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		c.String(http.StatusNotFound, "")
		return
	}
	c.Abort()
	if status == http.StatusOK {
		// ServeContent answers ranges and conditional requests, and sets the
		// type from the name.
		http.ServeContent(c.Writer, c.Request, st.Name(), st.ModTime(), f)
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(status)
	if c.Request.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(c.Writer, f)
}
