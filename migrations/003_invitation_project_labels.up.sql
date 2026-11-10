-- Follow the seed's lock order and drain project deletions before locking invitations.
LOCK TABLE users IN ACCESS SHARE MODE;
LOCK TABLE projects IN SHARE MODE;

ALTER TABLE invitations ADD COLUMN project_name text;

-- The invitation-time name is unknown. Retain the current name only when the
-- recipient may see it at migration time.
UPDATE invitations i
SET project_name = CASE
    WHEN i.status = 'pending' OR u.role = 'superadmin' OR EXISTS (
        SELECT 1 FROM project_members m
        WHERE m.project_id = i.project_id AND m.user_id = i.recipient_id
    ) THEN p.name
    ELSE i.project_id
END
FROM projects p, users u
WHERE p.id = i.project_id AND u.id = i.recipient_id;

ALTER TABLE invitations ALTER COLUMN project_name SET NOT NULL;
