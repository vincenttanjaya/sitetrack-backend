INSERT INTO projects (number,name,start_date,end_date,next_task_number)
SELECT sp.number,
 (ARRAY['Harbour','Wadi','Airport','Seafront','City Centre','Industrial Park','Oasis','Logistics Hub'])[(sp.number % 8)+1]
   || ' ' || (ARRAY['Utilities','Access Road','Extension','Maintenance','Foundations','Fit-out'])[(sp.number/8 % 6)+1]
   || ' Phase ' || sp.number,
 c.anchor - (sp.number*7 % 1240)::integer - 180,
 c.anchor - (sp.number*7 % 1240)::integer - 180 + 365 + (sp.number*11 % 630)::integer,
 sp.task_count + 1
FROM seed_projects sp CROSS JOIN seed_config c;
