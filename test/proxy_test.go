//go:build integration

package test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/hexfusion/fray/pkg/oci"
	"github.com/hexfusion/fray/pkg/registry"
	"github.com/hexfusion/fray/pkg/store"
)

func TestProxyStartup(t *testing.T) {
	require := require.New(t)

	dir := t.TempDir()
	l, err := store.Open(dir)
	require.NoError(err)

	client := oci.NewClient()
	server := registry.New(l, client, registry.DefaultOptions())

	ts := httptest.NewServer(server)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"/v2/", nil)
	require.NoError(err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(err)
	defer resp.Body.Close()

	require.Equal(http.StatusOK, resp.StatusCode)
	require.Equal("registry/2.0", resp.Header.Get("Docker-Distribution-API-Version"))
}
