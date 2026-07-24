package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/patchwork-toolkit/patchwork/internal/config"
)

// runHealthcheck probes the local /api/v1/health endpoint and exits 0 if the
// instance is serving and its database is reachable, 1 otherwise.
//
// This exists so the distroless runtime image can carry a Docker HEALTHCHECK.
// That image has no shell and no curl, so the conventional
// `CMD curl -f .../health` is unrunnable — but the app's own binary is
// already in the image and can be invoked in exec form.
//
// The port comes from the same config file the server reads, so the probe
// cannot drift from where the server actually listens. A config that fails
// to load is reported unhealthy: the server could not have started either.
func runHealthcheck(configPath string) {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: config: %v\n", err)
		os.Exit(1)
	}

	url := "http://127.0.0.1:" + cfg.Server.Port + "/api/v1/health"
	client := &http.Client{Timeout: 3 * time.Second}

	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	// The endpoint returns 503 when the database is unreachable, so the
	// status line alone is enough. Read the body for the failure message.
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		fmt.Fprintf(os.Stderr, "healthcheck: %s: %s %s\n", url, resp.Status, body)
		os.Exit(1)
	}

	io.Copy(io.Discard, io.LimitReader(resp.Body, 512))
	os.Exit(0)
}
