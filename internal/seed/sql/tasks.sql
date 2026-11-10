INSERT INTO tasks (project_id,number,name,start_date,end_date,status)
SELECT p.id, slot,
 (ARRAY['Install','Inspect','Survey','Reinforce','Commission','Pour','Excavate','Test','Repair','Assemble'])[(slot%10)+1]
   || ' ' || (ARRAY['slab','utilities','formwork','drainage','foundations','retaining wall','services','access road','roof deck','paving'])[(slot/10 % 10)+1]
   || ' — Work package ' || slot,
 c.anchor+((slot*73+sp.number*11)%850)::integer-650,
 c.anchor+((slot*73+sp.number*11)%850)::integer-650+4+((slot*31)%95)::integer,
 CASE WHEN slot<=sp.task_count*2/5 THEN 'done' WHEN slot<=sp.task_count*11/20 THEN 'in_progress' ELSE 'not_started' END
FROM seed_projects sp JOIN projects p USING (number) CROSS JOIN seed_config c
CROSS JOIN LATERAL generate_series(1,sp.task_count) slot;
