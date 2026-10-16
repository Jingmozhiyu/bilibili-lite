ALTER TABLE videos ADD COLUMN IF NOT EXISTS status VARCHAR(32) NOT NULL DEFAULT 'published';
ALTER TABLE videos ADD COLUMN IF NOT EXISTS failure_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE videos ADD COLUMN IF NOT EXISTS ready_at TIMESTAMPTZ;
ALTER TABLE videos ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_videos_status ON videos (status);
CREATE INDEX IF NOT EXISTS idx_videos_deleted_at ON videos (deleted_at);
CREATE INDEX IF NOT EXISTS idx_videos_published_feed ON videos (status, id DESC);

CREATE TABLE IF NOT EXISTS video_view_sessions (
    id VARCHAR(64) PRIMARY KEY,
    video_id BIGINT NOT NULL REFERENCES videos(id) ON UPDATE CASCADE ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    counted BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_view_session_video_user ON video_view_sessions (video_id, user_id);
CREATE INDEX IF NOT EXISTS idx_video_view_sessions_completed_at ON video_view_sessions (completed_at);
CREATE INDEX IF NOT EXISTS idx_view_count_limits ON video_view_sessions (user_id, video_id, counted, completed_at DESC);
