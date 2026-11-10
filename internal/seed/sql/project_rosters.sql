CREATE TEMP TABLE seed_projects ON COMMIT DROP AS
SELECT n AS number,
  CASE WHEN n=1 THEN c.largest_project
       ELSE (c.task_count-c.largest_project)/(c.project_count-1)
          + CASE WHEN n-2 < (c.task_count-c.largest_project)%(c.project_count-1) THEN 1 ELSE 0 END
  END AS task_count,
  CASE WHEN n=1 THEN c.max_members ELSE 4 + (n*13)%(c.max_members-3) END AS member_count,
  3 + ((n-1)*47)%(c.user_count-2) AS owner_id
FROM seed_config c CROSS JOIN generate_series(1,c.project_count) n;
