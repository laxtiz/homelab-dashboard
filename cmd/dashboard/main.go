package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"dashboard/internal/collector"
	"dashboard/internal/config"
	"dashboard/internal/server"
	"dashboard/internal/ws"
)

func main() {
	var (
		cfgPath string
		addr    string
		dev     bool
	)
	flag.StringVar(&cfgPath, "config", "", "path to config file (default: auto-resolve)")
	flag.StringVar(&addr, "addr", "", "override listen address")
	flag.BoolVar(&dev, "dev", false, "serve frontend from disk (web/dist) instead of embedded")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	path, err := config.Resolve(cfgPath)
	if err != nil {
		log.Fatalf("resolve config: %v", err)
	}

	hub := ws.New()

	initial, err := config.Load(path)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if addr != "" {
		initial.Server.Addr = addr
	}

	col, err := collector.New(initial, hub)
	if err != nil {
		log.Fatalf("init collector: %v", err)
	}

	mgr, err := config.NewManager(path,
		func(cfg *config.Config) { col.Reload(cfg) },
		func(ev config.ReloadEvent) { hub.Publish(ws.Message{Type: "reload", Data: ev}) },
	)
	if err != nil {
		log.Fatalf("config watcher: %v", err)
	}
	go func() {
		if err := mgr.Start(ctx); err != nil && ctx.Err() == nil {
			log.Printf("config watcher stopped: %v", err)
		}
	}()

	go col.Run(ctx)

	srv := server.New(hub, col, dev)
	e := srv.Router()
	go func() {
		<-ctx.Done()
		_ = e.Shutdown(context.Background())
	}()

	log.Printf("dashboard listening on %s (config %s, dev=%v)", initial.Server.Addr, path, dev)
	if err := e.Start(initial.Server.Addr); err != nil {
		log.Fatal(err)
	}
}
