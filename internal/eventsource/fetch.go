package eventsource

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/safehttp"
)

// maxFeedBytes bounds one feed document. A community calendar measured
// in megabytes is either broken or not a community calendar.
const maxFeedBytes = 2 << 20

// httpClient is SSRF-guarded: source URLs are admin-supplied, and
// fetching them must never probe the host's own network.
var httpClient = safehttp.NewClient(30 * time.Second)

type fetchResult struct {
	Body         []byte
	Etag         string
	LastModified string
	NotModified  bool
}

// fetchFeed retrieves a feed URL with conditional-GET state from the
// previous successful fetch, so an unchanged calendar costs one 304.
func fetchFeed(ctx context.Context, feedURL, etag, lastModified string) (*fetchResult, error) {
	if u, err := url.Parse(feedURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("source url must be http(s)")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/calendar, */*;q=0.5")
	req.Header.Set("User-Agent", "Patchwork/1.0 (event source)")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return &fetchResult{NotModified: true}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch feed: http %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read feed: %w", err)
	}
	if len(body) > maxFeedBytes {
		return nil, fmt.Errorf("feed exceeds %d bytes", maxFeedBytes)
	}

	return &fetchResult{
		Body:         body,
		Etag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}, nil
}
