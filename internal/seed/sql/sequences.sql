-- Restart identities transactionally after set-based inserts used explicit IDs.
DO $$
BEGIN
 EXECUTE format('ALTER TABLE users ALTER COLUMN id RESTART WITH %s',(SELECT max(id)+1 FROM users));
 EXECUTE format('ALTER TABLE projects ALTER COLUMN number RESTART WITH %s',(SELECT max(number)+1 FROM projects));
 EXECUTE format('ALTER TABLE materials ALTER COLUMN id RESTART WITH %s',(SELECT max(id)+1 FROM materials));
 EXECUTE format('ALTER TABLE deliveries ALTER COLUMN id RESTART WITH %s',(SELECT max(id)+1 FROM deliveries));
END
$$;

ANALYZE users,projects,project_members,invitations,tasks,task_dependencies,stores,materials,stock,deliveries,delivery_lines;
