package admin

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/thorved/ssh-vpn/backend/internal/config"
)

func NewHandler(cfg config.Config, store Store) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	handler := &handler{cfg: cfg, store: store}
	api := router.Group("/api/admin")
	api.GET("/overview", handler.overview)
	api.DELETE("/rooms/:room", handler.deleteRoom)
	api.DELETE("/connections/:id", handler.deleteConnection)

	router.NoRoute(gin.WrapH(handler.static()))
	return router
}

type handler struct {
	cfg   config.Config
	store Store
}

func (h *handler) overview(c *gin.Context) {
	c.JSON(http.StatusOK, h.store.Snapshot(
		h.cfg.PublicDomain,
		h.cfg.PublicSSHPort,
		h.cfg.AdminUser,
		h.cfg.AdminDashboardPort,
	))
}

func (h *handler) deleteRoom(c *gin.Context) {
	roomName := c.Param("room")
	roomName, err := url.PathUnescape(roomName)
	if err != nil || strings.TrimSpace(roomName) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "room is required"})
		return
	}
	roomName = strings.TrimSpace(roomName)
	if roomName == h.cfg.AdminUser {
		c.JSON(http.StatusBadRequest, gin.H{"error": "admin room cannot be deleted"})
		return
	}

	conns, removedPublishers := h.store.DeleteRoom(roomName)
	for _, conn := range conns {
		_ = conn.Close()
	}

	c.JSON(http.StatusOK, gin.H{
		"room":              roomName,
		"closedConnections": len(conns),
		"removedPublishers": removedPublishers,
	})
}

func (h *handler) deleteConnection(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "connection id is required"})
		return
	}

	result, exists, protected := h.store.DeleteConnection(id, h.cfg.AdminUser)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
		return
	}
	if protected {
		c.JSON(http.StatusBadRequest, gin.H{"error": "admin connection cannot be disconnected"})
		return
	}

	_ = result.Conn.Close()
	c.JSON(http.StatusOK, gin.H{
		"id":                result.ID,
		"room":              result.Room,
		"removedPublishers": result.RemovedPublishers,
	})
}

func (h *handler) static() http.Handler {
	fileServer := http.FileServer(http.Dir(h.cfg.WebStaticDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodHead}, ", "))
			writeStaticError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		cleanURLPath := strings.TrimPrefix(pathpkg.Clean("/"+r.URL.Path), "/")
		if isRSCRequest(r) && h.serveRSC(w, r, cleanURLPath) {
			return
		}

		staticPath := filepath.Join(h.cfg.WebStaticDir, filepath.FromSlash(cleanURLPath))
		if info, err := os.Stat(staticPath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		if cleanURLPath != "" && filepath.Ext(cleanURLPath) == "" {
			if h.serveStaticFile(w, r, cleanURLPath+".html") {
				return
			}
			if h.serveStaticFile(w, r, filepath.ToSlash(filepath.Join(cleanURLPath, "index.html"))) {
				return
			}
		}

		indexPath := filepath.Join(h.cfg.WebStaticDir, "index.html")
		if _, err := os.Stat(indexPath); err != nil {
			writeStaticError(w, http.StatusServiceUnavailable, "dashboard static files are not available")
			return
		}

		if strings.HasPrefix(r.URL.Path, "/_next/") || filepath.Ext(r.URL.Path) != "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, indexPath)
	})
}

func (h *handler) serveStaticFile(w http.ResponseWriter, r *http.Request, relativePath string) bool {
	fullPath := filepath.Join(h.cfg.WebStaticDir, filepath.FromSlash(relativePath))
	if info, err := os.Stat(fullPath); err != nil || info.IsDir() {
		return false
	}

	http.ServeFile(w, r, fullPath)
	return true
}

func (h *handler) serveRSC(w http.ResponseWriter, r *http.Request, cleanURLPath string) bool {
	if filepath.Ext(cleanURLPath) != "" {
		return false
	}

	txtPath := "index.txt"
	if cleanURLPath != "" {
		txtPath = cleanURLPath + ".txt"
	}

	fullPath := filepath.Join(h.cfg.WebStaticDir, filepath.FromSlash(txtPath))
	if info, err := os.Stat(fullPath); err != nil || info.IsDir() {
		return false
	}

	w.Header().Set("Content-Type", "text/x-component; charset=utf-8")
	http.ServeFile(w, r, fullPath)
	return true
}

func isRSCRequest(r *http.Request) bool {
	return r.Header.Get("RSC") == "1" || r.URL.Query().Has("_rsc")
}

func writeStaticError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":%q}`+"\n", message)
}
