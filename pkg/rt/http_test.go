package rt

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nooga/let-go/pkg/vm"
)

func TestHandlerResponseHeadersUseRawStrings(t *testing.T) {
	handlerFnValue, err := vm.NativeFnType.Wrap(func(_ []vm.Value) (vm.Value, error) {
		return vm.NewPersistentMap([]vm.Value{
			vm.Keyword("status"), vm.Int(302),
			vm.Keyword("headers"), vm.NewPersistentMap([]vm.Value{
				vm.Keyword("location"), vm.String("/browse"),
			}),
		}), nil
	})
	if err != nil {
		t.Fatalf("wrap handler: %v", err)
	}
	handlerFn := handlerFnValue.(vm.Fn)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	(&Handler{fn: handlerFn}).ServeHTTP(rec, req)

	if got := rec.Code; got != http.StatusFound {
		t.Fatalf("expected status %d, got %d", http.StatusFound, got)
	}
	if got := rec.Header().Get("Location"); got != "/browse" {
		t.Fatalf("expected unquoted Location header, got %q", got)
	}
}

func TestHandlerResponseHeadersUseRawStringKeys(t *testing.T) {
	handlerFnValue, err := vm.NativeFnType.Wrap(func(_ []vm.Value) (vm.Value, error) {
		return vm.NewPersistentMap([]vm.Value{
			vm.Keyword("headers"), vm.NewPersistentMap([]vm.Value{
				vm.String("X-Request-ID"), vm.String("abc123"),
			}),
			vm.Keyword("body"), vm.String("ok"),
		}), nil
	})
	if err != nil {
		t.Fatalf("wrap handler: %v", err)
	}
	handlerFn := handlerFnValue.(vm.Fn)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	rec := httptest.NewRecorder()
	(&Handler{fn: handlerFn}).ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "abc123" {
		t.Fatalf("expected string header key to be unquoted, got %q", got)
	}
}
