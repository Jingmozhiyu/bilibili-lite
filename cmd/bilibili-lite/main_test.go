package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoadBootstrapResolvesPrefixedEnvironment(t *testing.T) {
	values := map[string]string{
		"SERVER_HTTP_NETWORK": "tcp", "SERVER_HTTP_ADDR": "127.0.0.1:18000", "SERVER_HTTP_TIMEOUT": "120s",
		"SERVER_GRPC_NETWORK": "tcp", "SERVER_GRPC_ADDR": "127.0.0.1:19000", "SERVER_GRPC_TIMEOUT": "3s",
		"DATABASE_DRIVER": "postgres", "DATABASE_SOURCE": "test-dsn",
		"MEDIA_STORAGE_DIR": t.TempDir(), "MEDIA_UPLOAD_IDLE_TIMEOUT": "30s", "MEDIA_TRANSCODE_TIMEOUT": "3600s",
		"MEDIA_MAX_UPLOAD_BYTES": "1024", "MEDIA_MAX_COVER_BYTES": "512", "MEDIA_MAX_USER_STORAGE_BYTES": "4096",
		"MEDIA_TRANSCODE_WORKERS": "1", "MEDIA_TRANSCODE_POLL_INTERVAL": "1s",
		"SEARCH_ADDRESS": "http://127.0.0.1:17700", "SEARCH_API_KEY": "search-key", "SEARCH_VIDEO_INDEX": "videos-test", "SEARCH_RETRY_INTERVAL": "5s",
		"REDIS_ADDRESS": "127.0.0.1:16379", "REDIS_PASSWORD": "redis-key", "REDIS_DATABASE": "2", "REDIS_VIDEO_RANKING_KEY": "videos:hot:test", "REDIS_USER_RATE_LIMIT_KEY_PREFIX": "bili:user-rate:test",
		"SEED_ENABLED": "false", "SEED_PASSWORD": "not-used-in-test",
		"AUTH_ISSUER": "test", "AUTH_SECRET": "01234567890123456789012345678901", "AUTH_ACCESS_TTL": "7200s", "AUTH_REFRESH_TTL": "2592000s",
	}
	for key, value := range values {
		t.Setenv("BILI_"+key, value)
	}

	bootstrap, err := loadBootstrap(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatalf("loadBootstrap() error = %v", err)
	}
	if bootstrap.Server.GetHttp().GetAddr() != "127.0.0.1:18000" || bootstrap.Server.GetHttp().GetTimeout().AsDuration() != 2*time.Minute {
		t.Fatalf("HTTP configuration = %+v", bootstrap.Server.GetHttp())
	}
	if bootstrap.Data.GetMedia().GetTranscodeWorkers() != 1 || bootstrap.Data.GetMedia().GetMaxUserStorageBytes() != 4096 {
		t.Fatalf("media configuration = %+v", bootstrap.Data.GetMedia())
	}
	if bootstrap.Data.GetMedia().GetTranscodeTimeout().AsDuration() != time.Hour {
		t.Fatalf("transcode timeout = %s, want %s", bootstrap.Data.GetMedia().GetTranscodeTimeout().AsDuration(), time.Hour)
	}
	if bootstrap.Data.GetRedis().GetPassword() != "redis-key" || bootstrap.Data.GetRedis().GetUserRateLimitKeyPrefix() != "bili:user-rate:test" || bootstrap.Auth.GetIssuer() != "test" {
		t.Fatalf("external/auth configuration did not resolve environment values")
	}
}
