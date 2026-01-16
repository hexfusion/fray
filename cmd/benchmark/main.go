// Package main provides benchmarks for fray components.
package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/hexfusion/fray/pkg/merkle"
	"github.com/hexfusion/fray/pkg/oci"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "hash":
		benchmarkHash()
	case "blob":
		if len(os.Args) < 3 {
			fmt.Println("Usage: benchmark blob <blob-url>")
			fmt.Println("Example: benchmark blob https://quay.io/v2/prometheus/prometheus/blobs/sha256:...")
			os.Exit(1)
		}
		benchmarkBlob(os.Args[2])
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: benchmark <command> [args]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  hash        Compare SHA-256 vs xxHash64 performance")
	fmt.Println("  blob <url>  Benchmark chunked download overhead against a blob")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  benchmark hash")
	fmt.Println("  benchmark blob https://quay.io/v2/prometheus/prometheus/blobs/sha256:...")
}

func benchmarkHash() {
	sizes := []int{1, 10, 100, 500}

	fmt.Println("Hash benchmark (MB/s):")
	fmt.Println()
	fmt.Printf("%-8s %12s %12s %8s\n", "Size", "SHA-256", "xxHash64", "Speedup")
	fmt.Println("-----------------------------------------------")

	for _, sizeMB := range sizes {
		data := make([]byte, sizeMB*1024*1024)

		// SHA-256
		start := time.Now()
		for i := 0; i < 3; i++ {
			h := sha256.New()
			h.Write(data)
			h.Sum(nil)
		}
		sha256Time := time.Since(start) / 3
		sha256Speed := float64(sizeMB) / sha256Time.Seconds()

		// xxHash64
		start = time.Now()
		for i := 0; i < 3; i++ {
			xxhash.Sum64(data)
		}
		xxhashTime := time.Since(start) / 3
		xxhashSpeed := float64(sizeMB) / xxhashTime.Seconds()

		fmt.Printf("%-8s %10.0f %10.0f %8.1fx\n",
			fmt.Sprintf("%dMB", sizeMB),
			sha256Speed, xxhashSpeed, xxhashSpeed/sha256Speed)
	}

	// Estimate edge device impact
	fmt.Println()
	fmt.Println("Estimated 500MB layer hashing time:")
	fmt.Println()
	fmt.Printf("%-15s %12s %12s\n", "Device", "SHA-256", "xxHash64")
	fmt.Println("-------------------------------------------")

	// Scale factors relative to this machine's SHA-256 speed
	devices := []struct {
		name   string
		sha256 float64 // MB/s
	}{
		{"x86 (modern)", 1500},
		{"Pi 4", 150},
		{"Pi Zero", 30},
	}

	for _, d := range devices {
		sha256Time := 500.0 / d.sha256
		// xxHash is typically 15-20x faster
		xxhashTime := 500.0 / (d.sha256 * 17)
		fmt.Printf("%-15s %10.1fs %10.1fs\n", d.name, sha256Time, xxhashTime)
	}
}

func benchmarkBlob(url string) {
	ctx := context.Background()
	chunkSize := 1024 * 1024 // 1MB

	// Get blob size first
	fmt.Println("Fetching blob size...")
	f := oci.NewFetcher()
	size, err := f.HeadSize(ctx, url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting size: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Blob size: %d bytes (%.2f MB)\n", size, float64(size)/1024/1024)
	fmt.Printf("Chunk size: %d bytes (1 MB)\n", chunkSize)
	fmt.Printf("Chunks: %d\n\n", (int(size)+chunkSize-1)/chunkSize)

	// ====== NETWORK BENCHMARK ======
	fmt.Println("===== NETWORK OVERHEAD =====")

	// Single request
	fmt.Print("Single request download... ")
	start := time.Now()
	singleData, _ := fetchFull(ctx, url)
	singleNet := time.Since(start)
	fmt.Printf("%v (%.2f MB/s)\n", singleNet, float64(len(singleData))/singleNet.Seconds()/1024/1024)

	// Chunked sequential
	fmt.Print("Chunked sequential... ")
	start = time.Now()
	chunks, _ := fetchChunkedSeq(ctx, url, size, chunkSize)
	chunkedSeqNet := time.Since(start)
	fmt.Printf("%v (%.2f MB/s)\n", chunkedSeqNet, float64(size)/chunkedSeqNet.Seconds()/1024/1024)

	// Chunked parallel (4 workers)
	fmt.Print("Chunked parallel (4)... ")
	start = time.Now()
	chunks, _ = fetchChunkedParallel(ctx, url, size, chunkSize, 4)
	chunked4Net := time.Since(start)
	fmt.Printf("%v (%.2f MB/s)\n", chunked4Net, float64(size)/chunked4Net.Seconds()/1024/1024)

	// Chunked parallel (8 workers)
	fmt.Print("Chunked parallel (8)... ")
	start = time.Now()
	chunks, _ = fetchChunkedParallel(ctx, url, size, chunkSize, 8)
	chunked8Net := time.Since(start)
	fmt.Printf("%v (%.2f MB/s)\n", chunked8Net, float64(size)/chunked8Net.Seconds()/1024/1024)

	fmt.Println()

	// ====== CPU BENCHMARK ======
	fmt.Println("===== CPU OVERHEAD =====")

	// SHA256 of full blob
	fmt.Print("SHA256 full blob... ")
	start = time.Now()
	h := sha256.New()
	h.Write(singleData)
	_ = h.Sum(nil)
	sha256Full := time.Since(start)
	fmt.Printf("%v (%.2f MB/s)\n", sha256Full, float64(len(singleData))/sha256Full.Seconds()/1024/1024)

	// SHA256 per chunk + Merkle tree
	fmt.Print("SHA256 per chunk + Merkle... ")
	start = time.Now()
	tree := merkle.New(size, chunkSize)
	for i, chunk := range chunks {
		tree.SetChunk(i, chunk)
	}
	_ = tree.Root()
	merkleTime := time.Since(start)
	fmt.Printf("%v (%.2f MB/s)\n", merkleTime, float64(size)/merkleTime.Seconds()/1024/1024)

	fmt.Println()

	// ====== MEMORY BENCHMARK ======
	fmt.Println("===== MEMORY OVERHEAD =====")
	var m runtime.MemStats

	// Memory for full blob
	runtime.GC()
	runtime.ReadMemStats(&m)
	baseAlloc := m.Alloc
	data := make([]byte, size)
	copy(data, singleData)
	runtime.ReadMemStats(&m)
	fullMem := m.Alloc - baseAlloc
	fmt.Printf("Full blob in memory: %.2f MB\n", float64(fullMem)/1024/1024)
	data = nil

	// Memory for Merkle tree state
	runtime.GC()
	runtime.ReadMemStats(&m)
	baseAlloc = m.Alloc
	tree = merkle.New(size, chunkSize)
	for i, chunk := range chunks {
		tree.SetChunk(i, chunk)
	}
	runtime.ReadMemStats(&m)
	treeMem := m.Alloc - baseAlloc
	fmt.Printf("Merkle tree state: %.2f KB (%d hashes)\n", float64(treeMem)/1024, tree.NumChunks)

	fmt.Println()

	// ====== DISK BENCHMARK ======
	fmt.Println("===== DISK OVERHEAD =====")
	tmpDir, _ := os.MkdirTemp("", "fray-bench-*")
	defer os.RemoveAll(tmpDir)

	// Write full blob
	fmt.Print("Write full blob... ")
	start = time.Now()
	fullPath := tmpDir + "/full.blob"
	os.WriteFile(fullPath, singleData, 0644)
	writeFullTime := time.Since(start)
	fmt.Printf("%v (%.2f MB/s)\n", writeFullTime, float64(size)/writeFullTime.Seconds()/1024/1024)

	// Write chunks separately
	fmt.Print("Write chunks separately... ")
	start = time.Now()
	for i, chunk := range chunks {
		chunkPath := fmt.Sprintf("%s/chunk-%05d", tmpDir, i)
		os.WriteFile(chunkPath, chunk, 0644)
	}
	writeChunksTime := time.Since(start)
	fmt.Printf("%v (%.2f MB/s)\n", writeChunksTime, float64(size)/writeChunksTime.Seconds()/1024/1024)

	// Assemble from chunks
	fmt.Print("Assemble from chunks... ")
	start = time.Now()
	assembled, _ := os.Create(tmpDir + "/assembled.blob")
	for i := range chunks {
		chunkPath := fmt.Sprintf("%s/chunk-%05d", tmpDir, i)
		data, _ := os.ReadFile(chunkPath)
		assembled.Write(data)
	}
	assembled.Close()
	assembleTime := time.Since(start)
	fmt.Printf("%v (%.2f MB/s)\n", assembleTime, float64(size)/assembleTime.Seconds()/1024/1024)

	// Tree state file
	treePath := tmpDir + "/tree.json"
	tree.SaveToFile(treePath)
	info, _ := os.Stat(treePath)
	fmt.Printf("Tree state file: %d bytes\n", info.Size())

	fmt.Println()

	// ====== SUMMARY ======
	fmt.Println("===== SUMMARY =====")
	fmt.Printf("Network overhead (seq):  %.1f%% slower\n", float64(chunkedSeqNet-singleNet)/float64(singleNet)*100)
	fmt.Printf("Network overhead (4x):   %.1f%% slower\n", float64(chunked4Net-singleNet)/float64(singleNet)*100)
	fmt.Printf("Network overhead (8x):   %.1f%% slower\n", float64(chunked8Net-singleNet)/float64(singleNet)*100)
	fmt.Printf("CPU overhead (Merkle):   %.1f%% of hash time\n", float64(merkleTime)/float64(sha256Full)*100)
	fmt.Printf("Disk overhead (chunks):  %.1f%% slower\n", float64(writeChunksTime-writeFullTime)/float64(writeFullTime)*100)
	fmt.Printf("Disk overhead (assemble): +%v\n", assembleTime)
	fmt.Printf("Memory overhead (tree):  %.2f KB (vs %.2f MB blob)\n", float64(treeMem)/1024, float64(size)/1024/1024)
}

func fetchFull(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func fetchChunkedSeq(ctx context.Context, url string, totalSize int64, chunkSize int) ([][]byte, error) {
	f := oci.NewFetcher()
	numChunks := (int(totalSize) + chunkSize - 1) / chunkSize
	chunks := make([][]byte, numChunks)

	for i := 0; i < numChunks; i++ {
		start := int64(i) * int64(chunkSize)
		end := start + int64(chunkSize)
		if end > totalSize {
			end = totalSize
		}

		data, err := f.FetchRange(ctx, url, start, end)
		if err != nil {
			return nil, err
		}
		chunks[i] = data
	}

	return chunks, nil
}

func fetchChunkedParallel(ctx context.Context, url string, totalSize int64, chunkSize int, workers int) ([][]byte, error) {
	numChunks := (int(totalSize) + chunkSize - 1) / chunkSize
	chunks := make([][]byte, numChunks)

	type job struct {
		index int
		start int64
		end   int64
	}

	jobs := make(chan job, numChunks)
	results := make(chan struct {
		index int
		data  []byte
		err   error
	}, numChunks)

	// Start workers
	for w := 0; w < workers; w++ {
		go func() {
			f := oci.NewFetcher()
			for j := range jobs {
				data, err := f.FetchRange(ctx, url, j.start, j.end)
				results <- struct {
					index int
					data  []byte
					err   error
				}{j.index, data, err}
			}
		}()
	}

	// Send jobs
	for i := 0; i < numChunks; i++ {
		start := int64(i) * int64(chunkSize)
		end := start + int64(chunkSize)
		if end > totalSize {
			end = totalSize
		}
		jobs <- job{i, start, end}
	}
	close(jobs)

	// Collect results
	for i := 0; i < numChunks; i++ {
		r := <-results
		if r.err != nil {
			return nil, r.err
		}
		chunks[r.index] = r.data
	}

	return chunks, nil
}
