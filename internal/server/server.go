package server

import (
	"context"
	"strings"

	"github.com/google/wire"
)

// ProviderSet is server providers.
var ProviderSet = wire.NewSet(NewGRPCServer, NewHTTPServer)

func adminOperation(_ context.Context, operation string) bool {
	if !strings.HasPrefix(operation, "/video.v1.VideoService/") {
		return false
	}
	switch strings.TrimPrefix(operation, "/video.v1.VideoService/") {
	case "ListAdminVideos", "ListPendingReviewVideos", "GetAdminVideo", "GetReviewVideoPlay", "ApproveVideo", "RejectVideo", "TakeDownVideo", "DeleteAdminVideo":
		return true
	default:
		return false
	}
}
