package probe

import (
	"encoding/json"
	"net/http"
	"net/http/pprof"
	"sync"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Handler aggregates health checkers and mounts probe endpoints onto a mux.
type Handler struct {
	mu         sync.RWMutex
	liveness   map[string]Checker
	readiness  map[string]Checker
}

// NewHandler returns an empty Handler with no registered checkers.
func NewHandler() *Handler {
	return &Handler{
		liveness:  make(map[string]Checker),
		readiness: make(map[string]Checker),
	}
}

// RegisterLiveness adds a named liveness checker.
func (h *Handler) RegisterLiveness(name string, c Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.liveness[name] = c
}

// RegisterReadiness adds a named readiness checker.
func (h *Handler) RegisterReadiness(name string, c Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.readiness[name] = c
}

// Mount registers all probe routes on mux.
func (h *Handler) Mount(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.handleLiveness)
	mux.HandleFunc("/readyz", h.handleReadiness)
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}

type checkResult struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

func (h *Handler) handleLiveness(w http.ResponseWriter, r *http.Request) {
	h.runCheckers(w, r, h.liveness)
}

func (h *Handler) handleReadiness(w http.ResponseWriter, r *http.Request) {
	h.runCheckers(w, r, h.readiness)
}

func (h *Handler) runCheckers(w http.ResponseWriter, r *http.Request, checkers map[string]Checker) {
	h.mu.RLock()
	snapshot := make(map[string]Checker, len(checkers))
	for k, v := range checkers {
		snapshot[k] = v
	}
	h.mu.RUnlock()

	results := make(map[string]string, len(snapshot))
	allOK := true
	for name, c := range snapshot {
		if err := c.Check(r.Context()); err != nil {
			results[name] = err.Error()
			allOK = false
		} else {
			results[name] = "ok"
		}
	}

	res := checkResult{Checks: results}
	if allOK {
		res.Status = "ok"
		w.WriteHeader(http.StatusOK)
	} else {
		res.Status = "degraded"
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}
