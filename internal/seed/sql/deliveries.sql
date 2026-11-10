WITH delivery_data AS (
 SELECT (sp.number-1)*(c.delivery_count/c.project_count)+n AS id,p.id AS project_id,sp.owner_id,
  CASE WHEN n%4=0 THEN 'project_to_company' ELSE 'company_to_project' END AS direction,
  CASE
   WHEN n%4=1 THEN 'delivered'
   WHEN n>c.delivery_count/c.project_count-greatest(4,c.delivery_count/c.project_count/20)
    AND (sp.number+n)%4=0 THEN 'in_transit'
   WHEN (sp.number*7+n)%11=0 THEN 'cancelled'
   WHEN (sp.number*3+n)%7=0 THEN 'rejected'
   ELSE 'delivered'
  END AS status,
  (c.anchor::timestamp AT TIME ZONE 'UTC') - interval '15 hours'
    - (c.delivery_count/c.project_count-n)*interval '4 days' + (sp.number%60)*interval '1 minute' AS sent_at
 FROM seed_projects sp JOIN projects p USING (number) CROSS JOIN seed_config c
 CROSS JOIN LATERAL generate_series(1,c.delivery_count/c.project_count) n
)
INSERT INTO deliveries (id,project_id,direction,status,sent_at,sent_by,resolved_at,resolved_by)
SELECT id,project_id,direction,status,sent_at,
 CASE WHEN direction='company_to_project' THEN 2 ELSE owner_id END,
 CASE WHEN status<>'in_transit' THEN sent_at+interval '8 hours' END,
 CASE
  WHEN status='in_transit' THEN NULL
  WHEN (direction='company_to_project') = (status='cancelled') THEN 2
  ELSE owner_id
 END
FROM delivery_data;
