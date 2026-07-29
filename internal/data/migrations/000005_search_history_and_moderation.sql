ALTER TABLE users
    ADD COLUMN IF NOT EXISTS role VARCHAR(32) NOT NULL DEFAULT 'user';

CREATE INDEX IF NOT EXISTS idx_users_role ON users (role);

ALTER TABLE videos
    ADD COLUMN IF NOT EXISTS review_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS submitted_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS reviewed_by BIGINT;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'videos_reviewer_fk') THEN
        ALTER TABLE videos
            ADD CONSTRAINT videos_reviewer_fk FOREIGN KEY (reviewed_by)
            REFERENCES users(id) ON UPDATE CASCADE ON DELETE SET NULL;
    END IF;
END $$;

ALTER TABLE videos ALTER COLUMN status SET DEFAULT 'processing';

CREATE INDEX IF NOT EXISTS idx_videos_pending_review
    ON videos (submitted_at ASC, id ASC)
    WHERE status = 'pending_review' AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS user_video_watch_history (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    video_id BIGINT NOT NULL REFERENCES videos(id) ON UPDATE CASCADE ON DELETE CASCADE,
    watched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_video_watch_history_unique UNIQUE (user_id, video_id)
);

CREATE INDEX IF NOT EXISTS idx_user_video_watch_history_page
    ON user_video_watch_history (user_id, watched_at DESC, id DESC);
