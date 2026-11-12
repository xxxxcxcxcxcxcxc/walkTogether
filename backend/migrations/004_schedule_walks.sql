ALTER TABLE users ADD COLUMN IF NOT EXISTS availability_visibility TEXT NOT NULL DEFAULT 'friends_exact';
ALTER TABLE users ADD COLUMN IF NOT EXISTS walk_notifications_enabled BOOLEAN NOT NULL DEFAULT TRUE;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'users_availability_visibility_check' AND conrelid = 'users'::regclass
    ) THEN
        ALTER TABLE users ADD CONSTRAINT users_availability_visibility_check
            CHECK (availability_visibility IN ('friends_exact', 'selected_exact', 'busy_only'));
    END IF;
END
$$;

ALTER TABLE availability ADD COLUMN IF NOT EXISTS data_state TEXT NOT NULL DEFAULT 'unknown';
UPDATE availability
SET data_state = CASE WHEN is_available THEN 'free_all' ELSE 'busy_all' END
WHERE data_state = 'unknown';
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'availability_data_state_check' AND conrelid = 'availability'::regclass
    ) THEN
        ALTER TABLE availability ADD CONSTRAINT availability_data_state_check
            CHECK (data_state IN ('unknown', 'free_all', 'busy_all', 'custom'));
    END IF;
END
$$;

ALTER TABLE availability_rules ADD COLUMN IF NOT EXISTS starts_on DATE NOT NULL DEFAULT CURRENT_DATE;
ALTER TABLE availability_rules ADD COLUMN IF NOT EXISTS ends_on DATE;
ALTER TABLE availability_rules ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'free';
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'availability_rules_date_range_check' AND conrelid = 'availability_rules'::regclass
    ) THEN
        ALTER TABLE availability_rules ADD CONSTRAINT availability_rules_date_range_check
            CHECK (ends_on IS NULL OR ends_on >= starts_on);
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'availability_rules_kind_check' AND conrelid = 'availability_rules'::regclass
    ) THEN
        ALTER TABLE availability_rules ADD CONSTRAINT availability_rules_kind_check CHECK (kind IN ('free', 'busy'));
    END IF;
END
$$;
CREATE INDEX IF NOT EXISTS idx_availability_rules_active_range
    ON availability_rules (user_id, day_of_week, starts_on, ends_on)
    WHERE is_active = TRUE;

CREATE TABLE IF NOT EXISTS availability_intervals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    day DATE NOT NULL,
    starts_at TIME NOT NULL,
    ends_at TIME NOT NULL,
    kind TEXT NOT NULL DEFAULT 'free' CHECK (kind IN ('free', 'busy')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (ends_at > starts_at),
    FOREIGN KEY (user_id, day) REFERENCES availability(user_id, day) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_availability_intervals_owner_day
    ON availability_intervals (user_id, day, starts_at, ends_at);

CREATE TABLE IF NOT EXISTS availability_shares (
    owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    friend_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (owner_user_id, friend_user_id),
    CHECK (owner_user_id <> friend_user_id)
);

ALTER TABLE meetings ADD COLUMN IF NOT EXISTS duration_minutes INTEGER NOT NULL DEFAULT 60;
ALTER TABLE meetings ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE meetings ADD CONSTRAINT meetings_duration_minutes_check CHECK (duration_minutes BETWEEN 15 AND 720);
ALTER TABLE meetings ADD CONSTRAINT meetings_time_order_check CHECK (scheduled_end > scheduled_start);

CREATE TABLE IF NOT EXISTS meeting_time_options (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    meeting_id UUID NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    proposed_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scheduled_start TIMESTAMPTZ NOT NULL,
    scheduled_end TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'proposed' CHECK (status IN ('proposed', 'selected', 'rejected')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (scheduled_end > scheduled_start)
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_meeting_time_selected
    ON meeting_time_options (meeting_id) WHERE status = 'selected';
CREATE INDEX IF NOT EXISTS idx_meeting_time_options_meeting
    ON meeting_time_options (meeting_id, created_at);

CREATE TABLE IF NOT EXISTS meeting_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    meeting_id UUID NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL CHECK (event_type IN ('created', 'time_proposed', 'time_selected', 'participant_accepted', 'participant_rejected', 'rescheduled', 'cancelled', 'completed')),
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_meeting_history_meeting
    ON meeting_history (meeting_id, created_at);

CREATE TABLE IF NOT EXISTS favorite_places (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    address TEXT NOT NULL DEFAULT '',
    latitude DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_favorite_places_user ON favorite_places (user_id, created_at DESC);

ALTER TABLE notification_jobs ADD COLUMN IF NOT EXISTS dedupe_key TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS ux_notification_jobs_dedupe_key
    ON notification_jobs (dedupe_key) WHERE dedupe_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_notification_jobs_due
    ON notification_jobs (scheduled_for, created_at)
    WHERE status IN ('queued', 'processing');