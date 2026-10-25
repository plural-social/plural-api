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
