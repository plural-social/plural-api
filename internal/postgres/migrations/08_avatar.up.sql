ALTER TABLE account
	ADD COLUMN avatar_media_uuid UUID NULL,
	ADD CONSTRAINT account_avatar_media_fkey FOREIGN KEY (avatar_media_uuid) REFERENCES media (uuid) ON DELETE SET NULL;
