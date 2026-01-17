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

	"github.com/hexfusion/fray/internal/prune"
	"github.com/hexfusion/fray/internal/version"
	"github.com/hexfusion/fray/pkg/logging"
	"github.com/hexfusion/fray/pkg/oci"
	"github.com/hexfusion/fray/pkg/proxy"
	"github.com/hexfusion/fray/pkg/store"
)

const (
	rootCacheDir     = "/var/lib/containers/fray"
	rootlessCacheDir = ".local/share/containers/fray"
	cacheEnvVar      = "FRAY_CACHE_DIR"
)

func main() {
	log := logging.NewConsole()
	defer func() { _ = log.Sync() }()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "pull":
		cmdPull(log, os.Args[2:])
	case "proxy":
		cmdProxy(os.Args[2:])
	case "status":
		cmdStatus(log, os.Args[2:])
	case "prune":
		cmdPrune(log, os.Args[2:])
	case "version":
		cmdVersion(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
	default:
		log.Error("unknown command", zap.String("command", os.Args[1]))
		os.Exit(1)
	}
}

func defaultCacheDir() string {
	if dir := os.Getenv(cacheEnvVar); dir != "" {
		return dir
	}
	if os.Getuid() == 0 {
		return rootCacheDir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, rootlessCacheDir)
	}
	return "./fray-cache"
}

func printUsage() {
	fmt.Println("fray - edge-native OCI image puller")
	fmt.Println()
	fmt.Println("Usage: fray <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  pull     Pull image to OCI layout")
	fmt.Println("  proxy    Run pull-through caching proxy")
	fmt.Println("  status   Show layout status")
	fmt.Println("  prune    Remove incomplete downloads and temp files")
	fmt.Println("  version  Show version information")
	fmt.Println()
	fmt.Println("Run 'fray <command> -h' for command options")
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

func cmdPull(log logging.Logger, args []string) {
	fs := flag.NewFlagSet("pull", flag.ExitOnError)
	output := fs.String("o", defaultCacheDir(), "output directory")
	chunkSize := fs.Int("c", 1024*1024, "chunk size in bytes")
	parallel := fs.Int("p", 4, "parallel downloads")
	silent := fs.Bool("s", false, "silent mode, suppress progress output")

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

	var progress float64
	var done bool
	spinner := []rune{'|', '/', '-', '\\'}

	// spinner goroutine
	if !*silent {
		go func() {
			i := 0
			for !done {
				fmt.Printf("\r%d%% %c  ", int(progress), spinner[i%len(spinner)])
				i++
				time.Sleep(100 * time.Millisecond)
			}
		}()
	}

	opts := store.PullOptions{
		ChunkSize: *chunkSize,
		Parallel:  *parallel,
		OnProgress: func(current, total int, layerProgress float64) {
			progress = (float64(current) + layerProgress) / float64(total) * 100
		},
	}

	puller := store.NewPuller(l, client, log, opts)
	start := time.Now()

	result, err := puller.Pull(ctx, image)
	done = true
	if !*silent {
		fmt.Printf("\r100%%    \n") // clear spinner and show complete
	}
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
	dataDir := fs.String("d", defaultCacheDir(), "data directory")
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

	server := proxy.New(l, client, log, proxy.Options{
		ChunkSize: *chunkSize,
		Parallel:  *parallel,
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

func cmdStatus(log logging.Logger, args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	dir := defaultCacheDir()
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

func cmdPrune(log logging.Logger, args []string) {
	fs := flag.NewFlagSet("prune", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "show what would be deleted without deleting")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	dir := defaultCacheDir()
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}

	opts := prune.Options{
		DryRun: *dryRun,
		OnItem: func(item prune.Item) {
			if *dryRun {
				if item.IsDir {
					log.Info("would delete", zap.String("dir", item.Path), zap.Int64("bytes", item.Bytes), zap.Int("files", item.Files))
				} else {
					log.Info("would delete", zap.String("file", item.Path), zap.Int64("bytes", item.Bytes))
				}
			}
		},
		OnDelete: func(item prune.Item, err error) {
			if err != nil {
				log.Warn("failed to remove", zap.String("path", item.Path), zap.Error(err))
			} else {
				log.Debug("removed", zap.String("path", item.Path))
			}
		},
	}

	result, err := prune.Run(dir, opts)
	if err != nil {
		log.Error("prune failed", zap.String("path", dir), zap.Error(err))
		os.Exit(1)
	}

	if result == nil || result.Files == 0 {
		log.Info("nothing to prune")
		return
	}

	action := "pruned"
	if *dryRun {
		action = "would prune"
	}

	log.Info(action,
		zap.Int("files", result.Files),
		zap.Int64("bytes", result.Bytes),
		zap.String("human", prune.HumanBytes(result.Bytes)),
	)
}
