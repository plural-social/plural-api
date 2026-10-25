-- Latest full schema. Keep in sync with internal/postgres/migrations.

CREATE TABLE IF NOT EXISTS account (
	uuid UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
	username TEXT NOT NULL,
	email TEXT NOT NULL,
	password_hash TEXT NOT NULL,
	display_name TEXT NOT NULL DEFAULT '',
	bio TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	CONSTRAINT account_username_key UNIQUE (username),
	CONSTRAINT account_email_key UNIQUE (email)
);

CREATE TABLE IF NOT EXISTS session (
	token_hash BYTEA NOT NULL PRIMARY KEY,
	account_uuid UUID NOT NULL,
	user_agent TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	last_used_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	expires_at TIMESTAMPTZ NOT NULL,
	CONSTRAINT session_account_fkey FOREIGN KEY (account_uuid) REFERENCES account (uuid) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS session_account_uuid_idx ON session (account_uuid);

CREATE TABLE IF NOT EXISTS media (
	uuid UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
	account_uuid UUID NOT NULL,
	kind TEXT NOT NULL,
	status TEXT NOT NULL,
	content_type TEXT NOT NULL, -- content type of display_key
	display_key TEXT NOT NULL DEFAULT '',
	thumbnail_key TEXT NOT NULL DEFAULT '',
	original_key TEXT NOT NULL DEFAULT '', -- videos only, deleted after processing
	width INTEGER NOT NULL DEFAULT 0,
	height INTEGER NOT NULL DEFAULT 0,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	size_bytes BIGINT NOT NULL DEFAULT 0, -- size of display_key
	failure_reason TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	processed_at TIMESTAMPTZ NULL,
	CONSTRAINT media_account_fkey FOREIGN KEY (account_uuid) REFERENCES account (uuid) ON DELETE CASCADE,
	CONSTRAINT media_kind_check CHECK (kind IN ('image', 'video')),
	CONSTRAINT media_status_check CHECK (status IN ('processing', 'ready', 'failed'))
);
CREATE INDEX IF NOT EXISTS media_account_uuid_idx ON media (account_uuid);

CREATE TABLE IF NOT EXISTS post (
	uuid UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
	account_uuid UUID NOT NULL,
	media_uuid UUID NOT NULL,
	caption TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	deleted_at TIMESTAMPTZ NULL,
	CONSTRAINT post_account_fkey FOREIGN KEY (account_uuid) REFERENCES account (uuid) ON DELETE CASCADE,
	CONSTRAINT post_media_fkey FOREIGN KEY (media_uuid) REFERENCES media (uuid) ON DELETE RESTRICT,
	CONSTRAINT post_media_uuid_key UNIQUE (media_uuid)
);
CREATE INDEX IF NOT EXISTS post_created_idx ON post (created_at DESC, uuid DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS post_account_created_idx ON post (account_uuid, created_at DESC, uuid DESC) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS follow (
	follower_uuid UUID NOT NULL,
	followee_uuid UUID NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	CONSTRAINT follow_pkey PRIMARY KEY (follower_uuid, followee_uuid),
	CONSTRAINT follow_follower_fkey FOREIGN KEY (follower_uuid) REFERENCES account (uuid) ON DELETE CASCADE,
	CONSTRAINT follow_followee_fkey FOREIGN KEY (followee_uuid) REFERENCES account (uuid) ON DELETE CASCADE,
	CONSTRAINT follow_not_self_check CHECK (follower_uuid <> followee_uuid)
);
CREATE INDEX IF NOT EXISTS follow_followee_uuid_idx ON follow (followee_uuid);

CREATE TABLE IF NOT EXISTS post_like (
	post_uuid UUID NOT NULL,
	account_uuid UUID NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	CONSTRAINT post_like_pkey PRIMARY KEY (post_uuid, account_uuid),
	CONSTRAINT post_like_post_fkey FOREIGN KEY (post_uuid) REFERENCES post (uuid) ON DELETE CASCADE,
	CONSTRAINT post_like_account_fkey FOREIGN KEY (account_uuid) REFERENCES account (uuid) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS comment (
	uuid UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
	post_uuid UUID NOT NULL,
	account_uuid UUID NOT NULL,
	body TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	deleted_at TIMESTAMPTZ NULL,
	CONSTRAINT comment_post_fkey FOREIGN KEY (post_uuid) REFERENCES post (uuid) ON DELETE CASCADE,
	CONSTRAINT comment_account_fkey FOREIGN KEY (account_uuid) REFERENCES account (uuid) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS comment_post_created_idx ON comment (post_uuid, created_at DESC, uuid DESC) WHERE deleted_at IS NULL;

ALTER TABLE account
	ADD COLUMN IF NOT EXISTS avatar_media_uuid UUID NULL,
	ADD CONSTRAINT account_avatar_media_fkey FOREIGN KEY (avatar_media_uuid) REFERENCES media (uuid) ON DELETE SET NULL;

ALTER TABLE account
	ADD COLUMN role TEXT NOT NULL DEFAULT 'user',
	ADD COLUMN suspended_at TIMESTAMPTZ NULL,
	ADD CONSTRAINT account_role_check CHECK (role IN ('user', 'moderator', 'admin'));

CREATE TABLE IF NOT EXISTS block (
	blocker_uuid UUID NOT NULL,
	blocked_uuid UUID NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	CONSTRAINT block_pkey PRIMARY KEY (blocker_uuid, blocked_uuid),
	CONSTRAINT block_blocker_fkey FOREIGN KEY (blocker_uuid) REFERENCES account (uuid) ON DELETE CASCADE,
	CONSTRAINT block_blocked_fkey FOREIGN KEY (blocked_uuid) REFERENCES account (uuid) ON DELETE CASCADE,
	CONSTRAINT block_not_self_check CHECK (blocker_uuid <> blocked_uuid)
);
CREATE INDEX IF NOT EXISTS block_blocked_uuid_idx ON block (blocked_uuid);

CREATE TABLE IF NOT EXISTS report (
	uuid UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
	reporter_uuid UUID NOT NULL,
	target_type TEXT NOT NULL,
	target_uuid UUID NOT NULL,
	reason TEXT NOT NULL,
	details TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'open',
	resolved_by_uuid UUID NULL,
	resolution_note TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	resolved_at TIMESTAMPTZ NULL,
	CONSTRAINT report_reporter_fkey FOREIGN KEY (reporter_uuid) REFERENCES account (uuid) ON DELETE CASCADE,
	CONSTRAINT report_resolved_by_fkey FOREIGN KEY (resolved_by_uuid) REFERENCES account (uuid) ON DELETE SET NULL,
	CONSTRAINT report_target_type_check CHECK (target_type IN ('post', 'comment', 'account')),
	CONSTRAINT report_reason_check CHECK (reason IN ('spam', 'harassment', 'nudity', 'violence', 'illegal', 'other')),
	CONSTRAINT report_status_check CHECK (status IN ('open', 'resolved', 'dismissed'))
);
CREATE INDEX IF NOT EXISTS report_status_created_idx ON report (status, created_at DESC, uuid DESC);
