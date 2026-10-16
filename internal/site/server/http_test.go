package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminOperationMatchesOnlyAdministratorVideoRPCs(t *testing.T) {
	t.Parallel()

	for _, operation := range []string{
		"/video.v1.VideoService/ListAdminVideos",
		"/video.v1.VideoService/GetAdminVideo",
		"/video.v1.VideoService/GetReviewVideoPlay",
		"/video.v1.VideoService/DeleteAdminVideo",
	} {
		if !adminOperation(context.Background(), operation) {
			t.Errorf("adminOperation(%q) = false, want true", operation)
		}
	}
	for _, operation := range []string{
		"/video.v1.VideoService/GetVideo",
		"/user.v1.UserService/GetMe",
		"",
	} {
		if adminOperation(context.Background(), operation) {
			t.Errorf("adminOperation(%q) = true, want false", operation)
		}
	}
}

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
