package server

import (
	"io/fs"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"dashboard/internal/collector"
	"dashboard/internal/ws"
	"dashboard/web"
)

type Server struct {
	hub *ws.Hub
	col *collector.Collector
	dev bool
	app *echo.Echo
}

func New(hub *ws.Hub, col *collector.Collector, dev bool) *Server {
	return &Server{hub: hub, col: col, dev: dev}
}

func (s *Server) Router() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())

	e.GET("/ws", s.handleWS)
	e.GET("/api/snapshot", s.handleSnapshot)
	e.GET("/api/health", s.handleHealth)
	s.serveStatic(e)

	return e
}

func (s *Server) serveStatic(e *echo.Echo) {
	var dist fs.FS
	if s.dev {
		dist = os.DirFS("web/dist")
	} else {
		sub, err := fs.Sub(webassets.FS, "dist")
		if err != nil {
			e.Logger.Error("embedded frontend missing: ", err)
			return
		}
		dist = sub
	}
	e.GET("/*", func(c echo.Context) error {
		p := c.Param("*")
		if _, err := fs.Stat(dist, p); err != nil {
			p = "index.html"
		}
		http.ServeFileFS(c.Response(), c.Request(), dist, p)
		return nil
	})
}

func (s *Server) handleWS(c echo.Context) error {
	s.hub.Handle(c.Response(), c.Request())
	return nil
}

func (s *Server) handleSnapshot(c echo.Context) error {
	snap := s.hub.LastSnapshot()
	if snap == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "no snapshot yet"})
	}
	return c.JSON(http.StatusOK, snap)
}

func (s *Server) handleHealth(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
