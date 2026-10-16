ALTER TABLE video_comments
    ADD COLUMN IF NOT EXISTS root_id BIGINT,
    ADD COLUMN IF NOT EXISTS parent_id BIGINT,
    ADD COLUMN IF NOT EXISTS reply_to_user_id BIGINT,
    ADD COLUMN IF NOT EXISTS like_count BIGINT NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'video_comments_root_fk') THEN
        ALTER TABLE video_comments
            ADD CONSTRAINT video_comments_root_fk FOREIGN KEY (root_id)
            REFERENCES video_comments(id) ON UPDATE CASCADE ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'video_comments_parent_fk') THEN
        ALTER TABLE video_comments
            ADD CONSTRAINT video_comments_parent_fk FOREIGN KEY (parent_id)
            REFERENCES video_comments(id) ON UPDATE CASCADE ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'video_comments_reply_user_fk') THEN
        ALTER TABLE video_comments
            ADD CONSTRAINT video_comments_reply_user_fk FOREIGN KEY (reply_to_user_id)
            REFERENCES users(id) ON UPDATE CASCADE ON DELETE SET NULL;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'video_comments_like_count_nonnegative') THEN
        ALTER TABLE video_comments
            ADD CONSTRAINT video_comments_like_count_nonnegative CHECK (like_count >= 0);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_video_comments_roots
    ON video_comments (video_id, id DESC) WHERE parent_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_video_comments_replies
    ON video_comments (root_id, id ASC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_video_comments_history
    ON video_comments (user_id, id DESC) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS user_video_comment_likes (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    comment_id BIGINT NOT NULL REFERENCES video_comments(id) ON UPDATE CASCADE ON DELETE CASCADE,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_video_comment_likes_unique UNIQUE (user_id, comment_id)
);
CREATE INDEX IF NOT EXISTS idx_user_video_comment_likes_comment
    ON user_video_comment_likes (comment_id, active);
