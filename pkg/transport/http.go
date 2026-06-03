package transport

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog"
	itransport "github.com/stlimtat/bunshin-go/internal/transport"
)

// HTTPTransport serves workflows over HTTP.
//
// Endpoints:
//
//	POST /workflows/{id}         — synchronous execution
//	GET  /workflows/{id}/stream  — SSE streaming execution
//
// SSE event format:
//
//	data: {"type":"step_start","step_id":"A"}
//	data: {"type":"llm_token","token":"Hello"}
//	data: {"type":"done","output":{...}}
type HTTPTransport struct {
	mu           sync.Mutex
	addr         string
	server       *http.Server
	logger       zerolog.Logger
	pprofHandler http.Handler
}

// NewHTTPTransport constructs an HTTPTransport listening on addr (e.g. ":8080").
func NewHTTPTransport(addr string) *HTTPTransport {
	return &HTTPTransport{addr: addr, logger: zerolog.Nop()}
}

// WithLogger sets the logger for the transport.
func (t *HTTPTransport) WithLogger(logger zerolog.Logger) *HTTPTransport {
	t.logger = logger
	return t
}

// WithPprof enables /debug/pprof/* endpoints using the provided handler.
// Pass http.DefaultServeMux after importing _ "net/http/pprof" in your main package.
func (t *HTTPTransport) WithPprof(h http.Handler) *HTTPTransport {
	t.pprofHandler = h
	return t
}

func (t *HTTPTransport) Serve(ctx context.Context, handler WorkflowHandler) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/workflows/", func(w http.ResponseWriter, r *http.Request) {
		t.HandleRequest(w, r, handler)
	})

	mux.HandleFunc("/health", itransport.HandleHealth)
	mux.HandleFunc("/live", itransport.HandleLive)
	mux.HandleFunc("/ready", itransport.HandleReady)

	if t.pprofHandler != nil {
		mux.Handle("/debug/pprof/", t.pprofHandler)
	}

	srv := &http.Server{Addr: t.addr, Handler: mux}
	t.mu.Lock()
	t.server = srv
	t.mu.Unlock()

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			t.logger.Error().Err(err).Msg("graceful shutdown incomplete")
		}
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (t *HTTPTransport) Shutdown(ctx context.Context) error {
	t.mu.Lock()
	srv := t.server
	t.mu.Unlock()
	if srv != nil {
		return srv.Shutdown(ctx)
	}
	return nil
}

// HandleRequest dispatches a request to the sync or SSE handler.
// Exported for testing without starting a real HTTP server.
func (t *HTTPTransport) HandleRequest(w http.ResponseWriter, r *http.Request, h WorkflowHandler) {
	switch {
	case r.Method == http.MethodPost && !itransport.IsStreamPath(r.URL.Path):
		itransport.HandleSync(t.logger, w, r, h)
	case r.Method == http.MethodGet && itransport.IsStreamPath(r.URL.Path):
		itransport.HandleSSE(t.logger, w, r, h)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}
