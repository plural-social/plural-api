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
