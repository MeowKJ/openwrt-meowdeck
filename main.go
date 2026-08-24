package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/MeowKJ/openwrt-meowdeck/internal/config"
	"github.com/MeowKJ/openwrt-meowdeck/internal/discovery"
	"github.com/MeowKJ/openwrt-meowdeck/internal/history"
	"github.com/MeowKJ/openwrt-meowdeck/internal/server"
	"github.com/MeowKJ/openwrt-meowdeck/internal/updater"
	"github.com/MeowKJ/openwrt-meowdeck/internal/webui"
)

var version = "dev"

func main() {
	command := "serve"
	arguments := os.Args[1:]
	if len(arguments) > 0 {
		switch arguments[0] {
		case "serve", "update", "self-test", "version":
			command = arguments[0]
			arguments = arguments[1:]
		}
	}
	var err error
	switch command {
	case "serve":
		err = serve(arguments)
	case "update":
		err = update(arguments)
	case "self-test":
		err = selfTest(arguments)
	case "version":
		fmt.Println(version)
	}
	if err != nil {
		slog.Error("MeowDeck stopped", "command", command, "error", err)
		os.Exit(1)
	}
}

func serve(arguments []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := flags.String("config", "/etc/meowdeck/config.json", "configuration file")
	statePath := flags.String("state", "", "confirmed incident state file")
	listen := flags.String("listen", "", "listen address override")
	checkConfig := flags.Bool("check-config", false, "validate configuration and exit")
	printHostname := flags.Bool("print-hostname", false, "print the configured hostname and exit")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if *printHostname {
		fmt.Println(cfg.Hostname)
		return nil
	}
	if *checkConfig {
		return nil
	}
	if *listen != "" {
		cfg.Listen = *listen
	}
	resolvedStatePath := *statePath
	if resolvedStatePath == "" {
		if filepath.Dir(*configPath) == "/etc/meowdeck" {
			resolvedStatePath = "/etc/meowdeck/history.json"
		} else {
			resolvedStatePath = filepath.Join(os.TempDir(), "meowdeck-history.json")
		}
	}
	store, err := history.Open(resolvedStatePath)
	if err != nil {
		return err
	}
	source := discovery.NewSystemSource(discovery.ExecRunner{}, cfg.LANInterfaces)
	engine := discovery.NewEngine(version, source, discovery.NewHTTPScanner(), store)
	assets, err := webui.FS()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go engine.Run(ctx)
	application := server.New(cfg.Listen, engine, assets, version)
	go func() {
		slog.Info("MeowDeck started", "version", version, "listen", cfg.Listen, "hostname", cfg.Hostname, "lan_interfaces", cfg.LANInterfaces)
		if err := application.HTTPServer().ListenAndServe(); err != nil && err.Error() != "http: Server closed" {
			slog.Error("serve", "error", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := application.HTTPServer().Shutdown(shutdownContext); err != nil {
		return err
	}
	return store.Close()
}

func selfTest(arguments []string) error {
	flags := flag.NewFlagSet("self-test", flag.ContinueOnError)
	configPath := flags.String("config", "/etc/meowdeck/config.json", "configuration file")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if _, err := config.Load(*configPath); err != nil {
		return err
	}
	assets, err := webui.FS()
	if err != nil {
		return err
	}
	file, err := assets.Open("index.html")
	if err != nil {
		return err
	}
	return file.Close()
}

func update(arguments []string) error {
	flags := flag.NewFlagSet("update", flag.ContinueOnError)
	channel := flags.String("channel", envOr("MEOWDECK_CHANNEL", "stable"), "release channel")
	manifestURL := flags.String("manifest-url", "", "signed manifest URL")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	options := updater.DefaultOptions(*channel)
	if *manifestURL != "" {
		options.ManifestURL = *manifestURL
	}
	updated, err := updater.Run(context.Background(), options)
	if err != nil {
		return err
	}
	fmt.Printf("MeowDeck updated to %s (%s)\n", updated, *channel)
	return nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
