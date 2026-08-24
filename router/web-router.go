package router

import (
	"embed"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// WebAssets holds the embedded dashboard frontend assets.
type WebAssets struct {
	BuildFS   embed.FS
	IndexPage []byte
	// FrontendDir overrides the runtime frontend directory. An empty value uses
	// service.FrontendAssetsDir, allowing tests and non-container deployments to
	// provide an isolated directory.
	FrontendDir string
}

// frontendFileSystem overlays the downloaded frontend on top of the embedded
// assets. It resolves the directory on every request so a successful update is
// visible without restarting the process; missing files fall back to embed.
type frontendFileSystem struct {
	externalRoot string
	embedded     static.ServeFileSystem
}

func (f *frontendFileSystem) Exists(prefix, requestPath string) bool {
	if !service.ExternalFrontendEnabled() {
		return f.embedded.Exists(prefix, requestPath)
	}
	if externalPath, ok := f.externalPath(requestPath); ok {
		if info, err := os.Stat(externalPath); err == nil {
			if info.IsDir() {
				_, err = os.Stat(filepath.Join(externalPath, static.INDEX))
				if err != nil {
					return false
				}
			}
			return true
		}
	}
	return f.embedded.Exists(prefix, requestPath)
}

func (f *frontendFileSystem) Open(name string) (http.File, error) {
	if !service.ExternalFrontendEnabled() {
		return f.embedded.Open(name)
	}
	if externalPath, ok := f.externalPath(name); ok {
		if file, err := os.Open(externalPath); err == nil {
			return file, nil
		}
	}
	return f.embedded.Open(name)
}

func (f *frontendFileSystem) externalPath(requestPath string) (string, bool) {
	if strings.TrimSpace(f.externalRoot) == "" || requestPath == "/" || strings.ContainsRune(requestPath, 0) {
		return "", false
	}
	// URL paths use '/', including on platforms where filepath.Separator differs.
	clean := path.Clean("/" + strings.TrimPrefix(requestPath, "/"))
	rel := strings.TrimPrefix(clean, "/")
	if rel == "" || rel == "." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	root, err := filepath.Abs(f.externalRoot)
	if err != nil {
		return "", false
	}
	full, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", false
	}
	within, err := filepath.Rel(root, full)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) || filepath.IsAbs(within) {
		return "", false
	}
	// Do not follow a symlink planted inside the runtime directory. Extraction
	// rejects links, but this also protects operators who edit the directory
	// manually after an update.
	for current := full; ; current = filepath.Dir(current) {
		info, statErr := os.Lstat(current)
		if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", false
		}
		if current == root || filepath.Dir(current) == current {
			break
		}
	}
	return full, true
}

func SetWebRouter(router *gin.Engine, assets WebAssets) {
	embeddedFS := common.EmbedFolder(assets.BuildFS, "web/dist")
	frontendRoot := assets.FrontendDir
	if strings.TrimSpace(frontendRoot) == "" {
		frontendRoot = service.FrontendAssetsDir()
	}
	frontendFS := &frontendFileSystem{
		externalRoot: frontendRoot,
		embedded:     embeddedFS,
	}

	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.Use(middleware.GlobalWebRateLimit())
	router.Use(middleware.Cache())
	router.Use(static.Serve("/", frontendFS))
	router.NoRoute(func(c *gin.Context) {
		c.Set(middleware.RouteTagKey, "web")
		if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
			controller.RelayNotFound(c)
			return
		}
		c.Header("Cache-Control", "no-cache")
		indexPage := assets.IndexPage
		if service.ExternalFrontendEnabled() {
			if externalIndexPath, ok := frontendFS.externalPath("/" + static.INDEX); ok {
				if externalIndex, err := os.ReadFile(externalIndexPath); err == nil {
					indexPage = externalIndex
				}
			}
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexPage)
	})
}
