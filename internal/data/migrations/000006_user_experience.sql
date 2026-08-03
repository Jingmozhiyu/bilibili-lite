ALTER TABLE users
    ADD COLUMN IF NOT EXISTS experience BIGINT NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_experience_nonnegative') THEN
        ALTER TABLE users
            ADD CONSTRAINT users_experience_nonnegative CHECK (experience >= 0);
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS user_daily_experience (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON UPDATE CASCADE ON DELETE CASCADE,
    experience_date DATE NOT NULL,
    source VARCHAR(32) NOT NULL,
    amount INTEGER NOT NULL DEFAULT 0 CHECK (amount >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_daily_experience_unique UNIQUE (user_id, experience_date, source),
    CONSTRAINT user_daily_experience_source CHECK (source IN ('login', 'watch', 'share', 'coin'))
);

CREATE INDEX IF NOT EXISTS idx_user_daily_experience_date
    ON user_daily_experience (user_id, experience_date DESC);
