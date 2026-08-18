package worker

import "github.com/google/wire"

// ProviderSet is background worker providers.
var ProviderSet = wire.NewSet(NewUploadJanitor, NewVideoTranscoder, NewSearchIndexer, NewRecommendationRefresher)
