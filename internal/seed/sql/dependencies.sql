WITH bounds AS MATERIALIZED (
 SELECT t.project_id,sp.number AS project_number,t.number,
  CASE WHEN t.status='in_progress' THEN sp.task_count*2/5 ELSE t.number-1 END AS max_prerequisite
 FROM seed_projects sp JOIN projects p USING (number) JOIN tasks t ON t.project_id=p.id
), base AS (
 SELECT b.*, lane FROM bounds b CROSS JOIN LATERAL generate_series(1,least(2,b.max_prerequisite)) lane
), extras AS (
 SELECT b.*, 3 AS lane FROM bounds b WHERE b.max_prerequisite>=3
 ORDER BY b.project_number,b.number
 LIMIT (SELECT dependency_count-(SELECT sum(least(2,max_prerequisite)) FROM bounds) FROM seed_config)
), edges AS (
 SELECT * FROM base UNION ALL SELECT * FROM extras
)
INSERT INTO task_dependencies (project_id,task_number,prerequisite_number)
SELECT project_id, number, 1+(number*37+project_number*7+lane-1)%max_prerequisite FROM edges;
