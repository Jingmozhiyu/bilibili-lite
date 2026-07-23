package server

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLimitAvatarBodyRejectsOversizedRequest(t *testing.T) {
	t.Parallel()
	var readErr error
	handler := limitAvatarBody(4)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
	}))
	request := httptest.NewRequest(http.MethodPut, "/api/v1/users/me/avatar", strings.NewReader("12345"))

	handler.ServeHTTP(httptest.NewRecorder(), request)

	var maxBytesError *http.MaxBytesError
	if !errors.As(readErr, &maxBytesError) {
		t.Fatalf("read error = %v, want *http.MaxBytesError", readErr)
	}
}
