ALTER TABLE users
    ADD COLUMN IF NOT EXISTS coin_balance BIGINT NOT NULL DEFAULT 1000;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'users_coin_balance_nonnegative'
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT users_coin_balance_nonnegative CHECK (coin_balance >= 0);
    END IF;
END $$;

ALTER TABLE videos
    ADD COLUMN IF NOT EXISTS comment_count BIGINT NOT NULL DEFAULT 0;

ALTER TABLE user_video_coins
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ;
UPDATE user_video_coins
SET updated_at = COALESCE(updated_at, created_at, NOW())
WHERE updated_at IS NULL;
ALTER TABLE user_video_coins
    ALTER COLUMN updated_at SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_video_coin
    ON user_video_coins (user_id, video_id);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'user_video_coins_amount_limit'
    ) THEN
        ALTER TABLE user_video_coins
            ADD CONSTRAINT user_video_coins_amount_limit CHECK (amount BETWEEN 1 AND 2);
    END IF;
END $$;

ALTER TABLE user_video_shares
    ADD COLUMN IF NOT EXISTS request_id VARCHAR(64);
UPDATE user_video_shares
SET request_id = 'legacy-' || id::text
WHERE request_id IS NULL OR request_id = '';
ALTER TABLE user_video_shares
    ALTER COLUMN request_id SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_video_share_request
    ON user_video_shares (user_id, video_id, request_id);

CREATE INDEX IF NOT EXISTS idx_user_video_likes_history
    ON user_video_likes (user_id, active, updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_user_video_favorites_history
    ON user_video_favorites (user_id, active, updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_user_video_coins_history
    ON user_video_coins (user_id, updated_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS video_comments (
    id BIGSERIAL PRIMARY KEY,
    video_id BIGINT NOT NULL REFERENCES videos(id) ON UPDATE CASCADE ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    content VARCHAR(2000) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_video_comments_page
    ON video_comments (video_id, deleted_at, id DESC);
CREATE INDEX IF NOT EXISTS idx_video_comments_user_id
    ON video_comments (user_id);
