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
