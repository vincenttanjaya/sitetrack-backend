INSERT INTO project_members (project_id,user_id,is_admin,joined_at)
SELECT p.id, 3+((sp.number-1)*47+(slot-1)*3)%(c.user_count-2),
 slot=1 OR (slot=2 AND sp.number%3=0),
 CASE
  WHEN slot>1 AND slot<=c.invitation_count/c.project_count AND slot%5 IN (0,2)
  THEN (c.anchor::timestamp AT TIME ZONE 'UTC') - (c.invitation_count/c.project_count-slot+1)*interval '6 days' + interval '8 hours'
  ELSE (p.start_date::timestamp AT TIME ZONE 'UTC') - interval '30 days'
 END
FROM seed_projects sp JOIN projects p USING (number)
CROSS JOIN seed_config c CROSS JOIN LATERAL generate_series(1,sp.member_count) slot;
