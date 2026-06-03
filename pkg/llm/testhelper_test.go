package llm

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServer starts an httptest.Server and skips the test if the network is unavailable
// (e.g. in sandboxed CI environments that disallow port binding).
func newTestServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Skipf("network unavailable (httptest cannot bind): %v", r)
			}
		}()
		srv = httptest.NewServer(h)
	}()
	return srv
}
