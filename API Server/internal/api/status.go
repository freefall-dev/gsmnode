package api

import (
	"context"
	"io"
	"net/http"
	"sync"
	"time"
)

// healthClient is a short-timeout client for liveness probes, so a hung upstream
// cannot stall the request that triggered the probe.
var healthClient = &http.Client{Timeout: 4 * time.Second}

// svcHealth is one dependency's probe outcome.
type svcHealth struct {
	Status     string `json:"status"` // ok | error | unreachable
	URL        string `json:"url,omitempty"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
	LatencyMs  int64  `json:"latencyMs,omitempty"`
	Error      string `json:"error,omitempty"`
}

// probe issues a GET against url and classifies the result.
func probe(ctx context.Context, url string) svcHealth {
	h := svcHealth{URL: url}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		h.Status = "error"
		h.Error = err.Error()
		return h
	}
	start := time.Now()
	resp, err := healthClient.Do(req)
	h.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		h.Status = "unreachable"
		h.Error = err.Error()
		return h
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	h.HTTPStatus = resp.StatusCode
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		h.Status = "ok"
	} else {
		h.Status = "error"
	}
	return h
}

// handleStatus reports the health of this server and its neighbours (PocketBase
// and the Web App). Both are probed server-side and in parallel, so the browser
// never has to reach either directly and a slow upstream doesn't add to a fast
// one's latency.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	pbURL, _, _ := s.pbSettings()
	var pbHealth, web svcHealth
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); pbHealth = probe(r.Context(), pbURL+"/api/health") }()
	go func() { defer wg.Done(); web = probe(r.Context(), s.webAppURL()+"/healthz") }()
	wg.Wait()

	writeJSON(w, http.StatusOK, map[string]any{
		"apiServer":  svcHealth{Status: "ok"},
		"pocketBase": pbHealth,
		"webApp":     web,
	})
}
