package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/hexfusion/fray/internal/version"
	"github.com/hexfusion/fray/pkg/logging"
	"github.com/hexfusion/fray/pkg/oci"
	"github.com/hexfusion/fray/pkg/registry"
	"github.com/hexfusion/fray/pkg/store"
)

func main() {
	log := newConsoleLogger()
	defer func() { _ = log.Sync() }()

	if len(os.Args) < 2 {
		printUsage(log)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "pull":
		cmdPull(log, os.Args[2:])
	case "proxy":
		cmdProxy(os.Args[2:])
	case "status":
		cmdStatus(log, os.Args[2:])
	case "version":
		cmdVersion(os.Args[2:])
	case "help", "-h", "--help":
		printUsage(log)
	default:
		log.Error("unknown command", zap.String("command", os.Args[1]))
		os.Exit(1)
	}
}

func newConsoleLogger() *zap.Logger {
	cfg := zap.NewDevelopmentConfig()
	cfg.EncoderConfig.TimeKey = ""
	cfg.EncoderConfig.CallerKey = ""
	cfg.DisableStacktrace = true
	log, _ := cfg.Build()
	return log
}

func printUsage(log *zap.Logger) {
	log.Info("fray - edge-native OCI image puller",
		zap.String("usage", "fray <command> [options]"),
	)
	log.Info("commands",
		zap.String("pull", "pull image to OCI layout"),
		zap.String("proxy", "run pull-through caching proxy"),
		zap.String("status", "show layout status"),
		zap.String("version", "show version information"),
	)
	log.Info("run 'fray <command> -h' for command options")
}

func cmdVersion(args []string) {
	fs := flag.NewFlagSet("version", flag.ExitOnError)
	jsonOutput := fs.Bool("json", false, "output as JSON")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	info := version.Get()

	if *jsonOutput {
		data, _ := json.MarshalIndent(info, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("fray %s\n", info.String())
	fmt.Printf("  commit:    %s\n", info.Commit)
	fmt.Printf("  built:     %s\n", info.BuildDate)
	fmt.Printf("  state:     %s\n", info.GitTreeState)
	fmt.Printf("  go:        %s\n", info.GoVersion)
	fmt.Printf("  platform:  %s\n", info.Platform)
}

func cmdPull(log *zap.Logger, args []string) {
	fs := flag.NewFlagSet("pull", flag.ExitOnError)
	output := fs.String("o", "./oci-layout", "output directory")
	chunkSize := fs.Int("c", 1024*1024, "chunk size in bytes")
	parallel := fs.Int("p", 4, "parallel downloads")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if fs.NArg() < 1 {
		log.Error("image reference required")
		os.Exit(1)
	}

	image := fs.Arg(0)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	l, err := store.Open(*output)
	if err != nil {
		log.Error("open layout failed", zap.Error(err))
		os.Exit(1)
	}

	client := oci.NewClient()
	client.SetAuth(oci.NewRegistryAuth())

	log.Info("pulling",
		zap.String("image", image),
		zap.String("output", *output),
	)

	opts := store.PullOptions{
		ChunkSize: *chunkSize,
		Parallel:  *parallel,
		OnProgress: func(layer int, progress float64) {
			log.Debug("progress",
				zap.Int("layer", layer),
				zap.Float64("percent", progress*100),
			)
		},
	}

	puller := store.NewPuller(l, client, opts)
	start := time.Now()

	result, err := puller.Pull(ctx, image)
	if err != nil {
		log.Error("pull failed", zap.Error(err))
		os.Exit(1)
	}

	elapsed := time.Since(start)
	fields := []zap.Field{
		zap.String("digest", result.Digest),
		zap.Int("layers", result.Layers),
		zap.Int64("total_bytes", result.TotalSize),
		zap.Int64("downloaded_bytes", result.Downloaded),
		zap.Int64("cached_bytes", result.Cached),
		zap.Duration("elapsed", elapsed),
	}

	if result.Downloaded > 0 {
		speed := float64(result.Downloaded) / elapsed.Seconds()
		fields = append(fields, zap.Float64("bytes_per_sec", speed))
	}

	log.Info("pull complete", fields...)
}

func cmdProxy(args []string) {
	fs := flag.NewFlagSet("proxy", flag.ExitOnError)
	listen := fs.String("l", ":5000", "listen address")
	dataDir := fs.String("d", "./fray-cache", "data directory")
	chunkSize := fs.Int("c", 1024*1024, "chunk size in bytes")
	parallel := fs.Int("p", 4, "parallel downloads")
	logFile := fs.String("log-file", "", "log file path")
	logLevel := fs.String("log-level", "info", "log level")
	logMaxSize := fs.Int("log-max-size", 100, "max log file size in MB")
	logMaxBackups := fs.Int("log-max-backups", 3, "max rotated log files")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	logCfg := logging.Config{
		Level:      *logLevel,
		File:       *logFile,
		MaxSize:    *logMaxSize,
		MaxBackups: *logMaxBackups,
		MaxAge:     28,
		Compress:   true,
	}

	log, err := logging.New(logCfg)
	if err != nil {
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	l, err := store.Open(*dataDir)
	if err != nil {
		log.Error("open cache failed", zap.Error(err))
		os.Exit(1)
	}

	client := oci.NewClient()
	client.SetAuth(oci.NewRegistryAuth())

	server := registry.New(l, client, registry.Options{
		ChunkSize: *chunkSize,
		Parallel:  *parallel,
		Logger:    log,
	})

	httpServer := &http.Server{
		Addr:    *listen,
		Handler: server,
	}

	done := make(chan struct{})
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		log.Info("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(ctx); err != nil {
			log.Error("shutdown error", zap.Error(err))
		}
		close(done)
	}()

	log.Info("proxy starting",
		zap.String("listen", *listen),
		zap.String("cache", *dataDir),
		zap.Int("chunk_kb", *chunkSize/1024),
		zap.Int("parallel", *parallel),
	)

	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Error("server error", zap.Error(err))
		os.Exit(1)
	}

	<-done
}

func cmdStatus(log *zap.Logger, args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	dir := "./oci-layout"
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}

	l, err := store.Open(dir)
	if err != nil {
		log.Error("open layout failed", zap.Error(err))
		os.Exit(1)
	}

	index, err := l.GetIndex()
	if err != nil {
		log.Error("read index failed", zap.Error(err))
		os.Exit(1)
	}

	stats, err := l.GetStats()
	if err != nil {
		log.Error("get stats failed", zap.Error(err))
		os.Exit(1)
	}

	log.Info("layout",
		zap.String("path", dir),
		zap.Int("images", len(index.Manifests)),
		zap.Int("blobs", stats.BlobCount),
		zap.Int64("total_bytes", stats.TotalSize),
	)

	for _, m := range index.Manifests {
		name := m.Annotations["org.opencontainers.image.ref.name"]
		if name == "" {
			name = "(untagged)"
		}
		log.Info("image",
			zap.String("ref", name),
			zap.String("digest", m.Digest),
			zap.Int64("size", m.Size),
		)
	}

	stateDir := filepath.Join(dir, ".fray")
	if entries, err := os.ReadDir(stateDir); err == nil && len(entries) > 0 {
		for _, e := range entries {
			log.Info("in_progress", zap.String("state", e.Name()))
		}
	}
}
