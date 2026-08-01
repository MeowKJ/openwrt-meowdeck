package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MeowKJ/openwrt-meowdeck/internal/config"
	"github.com/MeowKJ/openwrt-meowdeck/internal/monitor"
	"github.com/MeowKJ/openwrt-meowdeck/internal/server"
	"github.com/MeowKJ/openwrt-meowdeck/internal/webui"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "/etc/meowdeck/config.json", "configuration file")
	listen := flag.String("listen", "", "listen address override")
	checkConfig := flag.Bool("check-config", false, "validate configuration and exit")
	printHostname := flag.Bool("print-hostname", false, "print the validated configured hostname and exit")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}
	if *listen != "" {
		cfg.Listen = *listen
	}
	if *printHostname {
		fmt.Println(cfg.Hostname)
		return
	}
	if *checkConfig {
		return
	}

	assets, err := webui.FS()
	if err != nil {
		slog.Error("load embedded frontend", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	manager := monitor.New(cfg, version)
	go manager.Run(ctx)

	app := server.New(cfg.Listen, manager, assets, cfg, *configPath)
	go func() {
		slog.Info("MeowDeck started", "version", version, "listen", cfg.Listen, "hostname", cfg.Hostname)
		if err := app.HTTPServer().ListenAndServe(); err != nil && err.Error() != "http: Server closed" {
			slog.Error("serve", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := app.HTTPServer().Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown", "error", err)
	}
}
