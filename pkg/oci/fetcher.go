package oci

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Fetcher fetches byte ranges from HTTP endpoints.
type Fetcher struct {
	client     *http.Client
	maxRetries int
	retryDelay time.Duration
}

// NewFetcher creates a Fetcher with default settings.
func NewFetcher() *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		maxRetries: 3,
		retryDelay: time.Second,
	}
}

// FetchRange fetches bytes [start, end) from the given URL.
func (f *Fetcher) FetchRange(ctx context.Context, url string, start, end int64) ([]byte, error) {
	var lastErr error

	for attempt := 0; attempt <= f.maxRetries; attempt++ {
		if attempt > 0 {
			delay := f.retryDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("fetch cancelled: %w", ctx.Err())
			case <-time.After(delay):
			}
		}

		data, err := f.fetchRangeOnce(ctx, url, start, end)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("failed after %d retries: %w", f.maxRetries+1, lastErr)
}

func (f *Fetcher) fetchRangeOnce(ctx context.Context, url string, start, end int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end-1))

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusPartialContent {
		expectedLen := end - start
		if int64(len(data)) != expectedLen {
			return nil, fmt.Errorf("expected %d bytes, got %d", expectedLen, len(data))
		}
	}

	return data, nil
}

// HeadSize returns the content-length of a resource via HEAD request.
func (f *Fetcher) HeadSize(ctx context.Context, url string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return 0, err
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	return resp.ContentLength, nil
}
