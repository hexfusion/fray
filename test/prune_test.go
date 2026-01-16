//go:build integration

package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrunePartialFiles(t *testing.T) {
	require := require.New(t)

	dir := t.TempDir()
	setupLayout(t, dir)

	// create partial files
	blobDir := filepath.Join(dir, "blobs", "sha256")
	require.NoError(os.WriteFile(filepath.Join(blobDir, "abc123.partial"), []byte("partial data"), 0644))
	require.NoError(os.WriteFile(filepath.Join(blobDir, "def456.partial"), []byte("more partial"), 0644))

	// run prune
	cmd := exec.Command("go", "run", "../cmd/fray", "prune", dir)
	output, err := cmd.CombinedOutput()
	require.NoError(err, string(output))

	// verify partial files are gone
	entries, err := os.ReadDir(blobDir)
	require.NoError(err)
	for _, e := range entries {
		require.False(strings.HasSuffix(e.Name(), ".partial"), "partial file should be removed: %s", e.Name())
	}
}

func TestPruneStateDirectories(t *testing.T) {
	require := require.New(t)

	dir := t.TempDir()
	setupLayout(t, dir)

	// create state directories with chunks
	stateDir := filepath.Join(dir, ".fray")
	require.NoError(os.MkdirAll(stateDir, 0755))

	layer1 := filepath.Join(stateDir, "layer1")
	require.NoError(os.MkdirAll(layer1, 0755))
	require.NoError(os.WriteFile(filepath.Join(layer1, "chunk-00000"), []byte("chunk"), 0644))
	require.NoError(os.WriteFile(filepath.Join(layer1, "tree.json"), []byte("{}"), 0644))

	layer2 := filepath.Join(stateDir, "layer2")
	require.NoError(os.MkdirAll(layer2, 0755))
	require.NoError(os.WriteFile(filepath.Join(layer2, "chunk-00000"), []byte("chunk"), 0644))

	// run prune
	cmd := exec.Command("go", "run", "../cmd/fray", "prune", dir)
	output, err := cmd.CombinedOutput()
	require.NoError(err, string(output))

	// verify state directories are gone
	entries, err := os.ReadDir(stateDir)
	require.NoError(err)
	require.Empty(entries, "state directories should be removed")
}

func TestPruneDryRun(t *testing.T) {
	require := require.New(t)

	dir := t.TempDir()
	setupLayout(t, dir)

	// create partial file
	blobDir := filepath.Join(dir, "blobs", "sha256")
	partialFile := filepath.Join(blobDir, "test.partial")
	require.NoError(os.WriteFile(partialFile, []byte("partial"), 0644))

	// run prune with dry-run
	cmd := exec.Command("go", "run", "../cmd/fray", "prune", "--dry-run", dir)
	output, err := cmd.CombinedOutput()
	require.NoError(err, string(output))
	require.Contains(string(output), "would delete")

	// verify file still exists
	_, err = os.Stat(partialFile)
	require.NoError(err, "partial file should still exist after dry-run")
}

func TestPruneNothingToClean(t *testing.T) {
	require := require.New(t)

	dir := t.TempDir()
	setupLayout(t, dir)

	// run prune on clean layout
	cmd := exec.Command("go", "run", "../cmd/fray", "prune", dir)
	output, err := cmd.CombinedOutput()
	require.NoError(err, string(output))
	require.Contains(string(output), "nothing to prune")
}

func TestPruneMissingDirectory(t *testing.T) {
	require := require.New(t)

	cmd := exec.Command("go", "run", "../cmd/fray", "prune", "/nonexistent/path")
	output, err := cmd.CombinedOutput()
	require.Error(err)
	require.Contains(string(output), "no such file or directory")
}

func TestPruneEnvVar(t *testing.T) {
	require := require.New(t)

	dir := t.TempDir()
	setupLayout(t, dir)

	// create partial file
	blobDir := filepath.Join(dir, "blobs", "sha256")
	require.NoError(os.WriteFile(filepath.Join(blobDir, "envtest.partial"), []byte("data"), 0644))

	// run prune using env var for default dir
	cmd := exec.Command("go", "run", "../cmd/fray", "prune")
	cmd.Env = append(os.Environ(), "FRAY_CACHE_DIR="+dir)
	output, err := cmd.CombinedOutput()
	require.NoError(err, string(output))
	require.Contains(string(output), "pruned")
}

func setupLayout(t *testing.T, dir string) {
	t.Helper()
	require := require.New(t)

	require.NoError(os.MkdirAll(filepath.Join(dir, "blobs", "sha256"), 0755))
	require.NoError(os.WriteFile(filepath.Join(dir, "oci-layout"), []byte(`{"imageLayoutVersion":"1.0.0"}`), 0644))
	require.NoError(os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{"schemaVersion":2,"manifests":[]}`), 0644))
}
