ALTER TABLE videos ADD COLUMN IF NOT EXISTS upload_job_id VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE videos ADD COLUMN IF NOT EXISTS upload_bytes BIGINT NOT NULL DEFAULT 0;
ALTER TABLE videos ADD COLUMN IF NOT EXISTS processing_started_at TIMESTAMPTZ;
ALTER TABLE videos ADD COLUMN IF NOT EXISTS processing_attempts INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_videos_processing_queue
    ON videos (status, processing_started_at, id)
    WHERE status = 'processing' AND upload_job_id <> '';

CREATE TABLE IF NOT EXISTS video_search_outbox (
    video_id BIGINT PRIMARY KEY REFERENCES videos(id) ON UPDATE CASCADE ON DELETE CASCADE,
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    locked_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_video_search_outbox_available
    ON video_search_outbox (available_at, video_id);

CREATE OR REPLACE FUNCTION enqueue_video_search_outbox()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO video_search_outbox (video_id, available_at, locked_at, last_error, updated_at)
    VALUES (NEW.id, NOW(), NULL, '', NOW())
    ON CONFLICT (video_id) DO UPDATE SET
        attempts = 0,
        available_at = NOW(),
        locked_at = NULL,
        last_error = '',
        updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS videos_search_outbox_trigger ON videos;
CREATE TRIGGER videos_search_outbox_trigger
AFTER INSERT OR UPDATE OF title, description, tags, status, owner_id,
    view_count, like_count, favorite_count, danmaku_count, comment_count, publish_time, deleted_at
ON videos
FOR EACH ROW EXECUTE FUNCTION enqueue_video_search_outbox();

INSERT INTO video_search_outbox (video_id)
SELECT id FROM videos
ON CONFLICT (video_id) DO NOTHING;
