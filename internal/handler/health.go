package handler

import (
	"encoding/json"
	"net/http"

	"github.com/patchwork-toolkit/patchwork/internal/config"
	"github.com/patchwork-toolkit/patchwork/internal/database"
)

// Version is set at build time via -ldflags.
var Version = "dev"

type HealthResponse struct {
	Status         string `json:"status"`
	DBStatus       string `json:"db_status"`
	ConfigLoaded   bool   `json:"config_loaded"`
	SMTPConfigured bool   `json:"smtp_configured"`
	Version        string `json:"version"`
}

// Health returns a handler that reports system status.
func Health(db *database.DB, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := HealthResponse{
			Status:         "ok",
			ConfigLoaded:   cfg != nil,
			SMTPConfigured: cfg != nil && cfg.SMTP.Configured(),
			Version:        Version,
		}

		// Check DB is reachable.
		code := http.StatusOK
		if err := db.Ping(); err != nil {
			resp.Status = "degraded"
			resp.DBStatus = "unreachable"
			// Degraded must be a non-2xx: external uptime probes and container
			// healthchecks read the status code, not the body. Returning 200
			// here reports "healthy" straight through a DB outage.
			code = http.StatusServiceUnavailable
		} else {
			resp.DBStatus = "ok"
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(resp)
	}
}
