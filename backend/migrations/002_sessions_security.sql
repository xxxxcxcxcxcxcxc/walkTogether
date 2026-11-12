ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT;
UPDATE users SET role = 'user' WHERE role IS NULL;
ALTER TABLE users ALTER COLUMN role SET DEFAULT 'user';
ALTER TABLE users ALTER COLUMN role SET NOT NULL;

DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1 FROM pg_constraint
		WHERE conname = 'users_role_check' AND conrelid = 'users'::regclass
	) THEN
		ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'admin'));
	END IF;
END
$$;

ALTER TABLE invitations ADD COLUMN IF NOT EXISTS accepted_at TIMESTAMPTZ;
ALTER TABLE invitations ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS ux_sessions_token_hash ON sessions (token_hash);
CREATE INDEX IF NOT EXISTS idx_sessions_user_revoked ON sessions (user_id, revoked_at);
CREATE INDEX IF NOT EXISTS idx_users_role ON users (role);
