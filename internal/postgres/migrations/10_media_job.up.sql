CREATE TABLE IF NOT EXISTS media_job (
	uuid UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
	media_uuid UUID NOT NULL,
	status TEXT NOT NULL DEFAULT 'queued',
	attempts INTEGER NOT NULL DEFAULT 0,
	run_after TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	locked_at TIMESTAMPTZ NULL,
	last_error TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	CONSTRAINT media_job_media_fkey FOREIGN KEY (media_uuid) REFERENCES media (uuid) ON DELETE CASCADE,
	CONSTRAINT media_job_media_uuid_key UNIQUE (media_uuid),
	CONSTRAINT media_job_status_check CHECK (status IN ('queued', 'running', 'done', 'failed'))
);
CREATE INDEX IF NOT EXISTS media_job_claim_idx ON media_job (status, run_after);
