CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(64) NOT NULL,
    password_hash VARCHAR(100) NOT NULL,
    display_name VARCHAR(100) NOT NULL,
    avatar_url VARCHAR(500),
    bio VARCHAR(500),
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username ON users (username);

CREATE TABLE IF NOT EXISTS videos (
    id BIGSERIAL PRIMARY KEY,
    owner_id BIGINT NOT NULL REFERENCES users(id) ON UPDATE CASCADE ON DELETE RESTRICT,
    title VARCHAR(200) NOT NULL,
    description TEXT,
    cover_url VARCHAR(500),
    duration_seconds BIGINT NOT NULL DEFAULT 0,
    view_count BIGINT NOT NULL DEFAULT 0,
    danmaku_count BIGINT NOT NULL DEFAULT 0,
    like_count BIGINT NOT NULL DEFAULT 0,
    coin_count BIGINT NOT NULL DEFAULT 0,
    favorite_count BIGINT NOT NULL DEFAULT 0,
    share_count BIGINT NOT NULL DEFAULT 0,
    publish_time TIMESTAMPTZ,
    tags JSONB,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_videos_owner_id ON videos (owner_id);

CREATE TABLE IF NOT EXISTS video_streams (
    id BIGSERIAL PRIMARY KEY,
    video_id BIGINT NOT NULL REFERENCES videos(id) ON UPDATE CASCADE ON DELETE CASCADE,
    stream_key VARCHAR(64) NOT NULL,
    label VARCHAR(32) NOT NULL,
    codec VARCHAR(64) NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    url VARCHAR(1000) NOT NULL,
    width INTEGER,
    height INTEGER,
    bandwidth INTEGER,
    default_stream BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_video_stream ON video_streams (video_id, stream_key);

CREATE TABLE IF NOT EXISTS danmakus (
    id BIGSERIAL PRIMARY KEY,
    video_id BIGINT NOT NULL REFERENCES videos(id) ON UPDATE CASCADE ON DELETE CASCADE,
    user_id BIGINT REFERENCES users(id) ON UPDATE CASCADE ON DELETE SET NULL,
    time_seconds DOUBLE PRECISION NOT NULL,
    text VARCHAR(500) NOT NULL,
    color VARCHAR(16) NOT NULL DEFAULT '#ffffff',
    created_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_danmakus_video_id ON danmakus (video_id);
CREATE INDEX IF NOT EXISTS idx_danmakus_user_id ON danmakus (user_id);

CREATE TABLE IF NOT EXISTS user_video_likes (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    video_id BIGINT NOT NULL REFERENCES videos(id) ON UPDATE CASCADE ON DELETE CASCADE,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_video_like ON user_video_likes (user_id, video_id);
CREATE INDEX IF NOT EXISTS idx_user_video_likes_video_id ON user_video_likes (video_id);

CREATE TABLE IF NOT EXISTS user_video_favorites (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    video_id BIGINT NOT NULL REFERENCES videos(id) ON UPDATE CASCADE ON DELETE CASCADE,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_video_favorite ON user_video_favorites (user_id, video_id);
CREATE INDEX IF NOT EXISTS idx_user_video_favorites_video_id ON user_video_favorites (video_id);

CREATE TABLE IF NOT EXISTS user_video_coins (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    video_id BIGINT NOT NULL REFERENCES videos(id) ON UPDATE CASCADE ON DELETE CASCADE,
    amount INTEGER NOT NULL CHECK (amount > 0),
    created_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_user_video_coins_user_id ON user_video_coins (user_id);
CREATE INDEX IF NOT EXISTS idx_user_video_coins_video_id ON user_video_coins (video_id);

CREATE TABLE IF NOT EXISTS user_video_shares (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    video_id BIGINT NOT NULL REFERENCES videos(id) ON UPDATE CASCADE ON DELETE CASCADE,
    created_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_user_video_shares_user_id ON user_video_shares (user_id);
CREATE INDEX IF NOT EXISTS idx_user_video_shares_video_id ON user_video_shares (video_id);
