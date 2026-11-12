CREATE EXTENSION IF NOT EXISTS pg_trgm;

ALTER TABLE users ADD COLUMN IF NOT EXISTS public_id UUID;
UPDATE users SET public_id = gen_random_uuid() WHERE public_id IS NULL;
ALTER TABLE users ALTER COLUMN public_id SET DEFAULT gen_random_uuid();
ALTER TABLE users ALTER COLUMN public_id SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_users_public_id ON users (public_id);

ALTER TABLE users ADD COLUMN IF NOT EXISTS friend_notifications_enabled BOOLEAN NOT NULL DEFAULT TRUE;
UPDATE users SET privacy_mode = 'all' WHERE privacy_mode = 'friends';
ALTER TABLE users ALTER COLUMN privacy_mode SET DEFAULT 'all';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'users_privacy_mode_check' AND conrelid = 'users'::regclass
    ) THEN
        ALTER TABLE users ADD CONSTRAINT users_privacy_mode_check
            CHECK (privacy_mode IN ('all', 'friends', 'hidden'));
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_users_display_name_trgm
    ON users USING GIN (display_name gin_trgm_ops)
    WHERE is_active = TRUE AND is_deleted = FALSE;
CREATE INDEX IF NOT EXISTS idx_users_username_trgm
    ON users USING GIN (username gin_trgm_ops)
    WHERE username IS NOT NULL AND is_active = TRUE AND is_deleted = FALSE;

CREATE TABLE IF NOT EXISTS friend_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_low_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_high_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    requester_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'rejected', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    responded_at TIMESTAMPTZ,
    ended_at TIMESTAMPTZ,
    CHECK (user_low_id < user_high_id),
    CHECK (requester_user_id <> recipient_user_id),
    CHECK (
        (requester_user_id = user_low_id AND recipient_user_id = user_high_id)
        OR (requester_user_id = user_high_id AND recipient_user_id = user_low_id)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_friend_requests_active_pair
    ON friend_requests (user_low_id, user_high_id)
    WHERE status IN ('pending', 'accepted');
CREATE INDEX IF NOT EXISTS idx_friend_requests_recipient_status
    ON friend_requests (recipient_user_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_friend_requests_requester_status
    ON friend_requests (requester_user_id, status, created_at DESC);

ALTER TABLE notification_jobs DROP CONSTRAINT IF EXISTS notification_jobs_status_check;
ALTER TABLE notification_jobs ADD CONSTRAINT notification_jobs_status_check
    CHECK (status IN ('queued', 'processing', 'sent', 'failed', 'cancelled'));
CREATE INDEX IF NOT EXISTS idx_notification_jobs_claim
    ON notification_jobs (created_at)
    WHERE status IN ('queued', 'processing');