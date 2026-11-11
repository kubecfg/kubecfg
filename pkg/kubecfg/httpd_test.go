package kubecfg

import (
	_ "expvar" // adds /debug/vars to http.DefaultServeMux
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/go-jsonnet"
)

func TestHttpdServesOnlyHooks(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("hook.jsonnet", []byte("function(request) {got: request.x}"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := hooks(func() (*jsonnet.VM, error) { return jsonnet.MakeVM(), nil }, []string{"hook.jsonnet"})

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/hook", strings.NewReader(`{"x": 1}`)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"got": 1`) {
		t.Errorf("POST /hook: %d %q", w.Code, w.Body)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/debug/vars", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("GET /debug/vars: got %d, want 404", w.Code)
	}
}
