package oci

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewFetcher(t *testing.T) {
	require := require.New(t)

	f := NewFetcher()

	require.NotNil(f.client)
	require.Equal(3, f.maxRetries)
}

func TestFetchRange(t *testing.T) {
	require := require.New(t)

	content := "0123456789abcdefghij"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rangeHeader := r.Header.Get("Range")
		require.NotEmpty(rangeHeader)

		// parse "bytes=start-end"
		var start, end int
		parseRange(rangeHeader, &start, &end)

		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte(content[start : end+1]))
	}))
	defer server.Close()

	f := NewFetcher()
	ctx := context.Background()

	data, err := f.FetchRange(ctx, server.URL, 0, 5)
	require.NoError(err)
	require.Equal("01234", string(data))

	data, err = f.FetchRange(ctx, server.URL, 5, 10)
	require.NoError(err)
	require.Equal("56789", string(data))

	data, err = f.FetchRange(ctx, server.URL, 10, 20)
	require.NoError(err)
	require.Equal("abcdefghij", string(data))
}

func TestFetchRangeFullResponse(t *testing.T) {
	require := require.New(t)

	content := "full content response"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// server ignores range, returns full content
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(content))
	}))
	defer server.Close()

	f := NewFetcher()
	ctx := context.Background()

	data, err := f.FetchRange(ctx, server.URL, 0, 10)
	require.NoError(err)
	require.Equal(content, string(data))
}

func TestFetchRangeError(t *testing.T) {
	require := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	f := NewFetcher()
	f.maxRetries = 0
	ctx := context.Background()

	_, err := f.FetchRange(ctx, server.URL, 0, 10)
	require.Error(err)
	require.Contains(err.Error(), "unexpected status: 500")
}

func TestHeadSize(t *testing.T) {
	require := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(http.MethodHead, r.Method)
		w.Header().Set("Content-Length", "12345")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	f := NewFetcher()
	ctx := context.Background()

	size, err := f.HeadSize(ctx, server.URL)
	require.NoError(err)
	require.Equal(int64(12345), size)
}

func TestHeadSizeError(t *testing.T) {
	require := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	f := NewFetcher()
	ctx := context.Background()

	_, err := f.HeadSize(ctx, server.URL)
	require.Error(err)
	require.Contains(err.Error(), "404")
}

func TestFetchRangeCancellation(t *testing.T) {
	require := require.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	f := NewFetcher()
	f.maxRetries = 5

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := f.FetchRange(ctx, server.URL, 0, 10)
	require.Error(err)
}

func parseRange(header string, start, end *int) {
	*start = 0
	*end = 0
	i := 0

	for i < len(header) && header[i] != '=' {
		i++
	}
	i++

	for i < len(header) && header[i] >= '0' && header[i] <= '9' {
		*start = *start*10 + int(header[i]-'0')
		i++
	}

	i++

	for i < len(header) && header[i] >= '0' && header[i] <= '9' {
		*end = *end*10 + int(header[i]-'0')
		i++
	}
}
