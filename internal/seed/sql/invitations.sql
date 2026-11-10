WITH invitation_data AS (
 SELECT p.id AS project_id, sp.number, sp.owner_id AS sender_id, slot,
  CASE
   WHEN sp.number=1 AND slot=1 THEN 4
   WHEN slot>1 AND slot<=sp.member_count AND slot%5 IN (0,2)
    THEN 3+((sp.number-1)*47+(slot-1)*3)%(c.user_count-2)
   ELSE 3+((sp.number-1)*47+(sp.member_count+slot-1)*3)%(c.user_count-2)
  END AS recipient_id,
  CASE
   WHEN slot>1 AND slot<=sp.member_count AND slot%5 IN (0,2) THEN 'accepted'
   WHEN slot%5 IN (1,2) THEN 'pending'
   WHEN slot%5 IN (0,3) THEN 'rejected'
   ELSE 'withdrawn'
  END AS status,
  (c.anchor::timestamp AT TIME ZONE 'UTC') - (c.invitation_count/c.project_count-slot+1)*interval '6 days' AS created_at
 FROM seed_projects sp JOIN projects p USING (number) CROSS JOIN seed_config c
 CROSS JOIN LATERAL generate_series(1,c.invitation_count/c.project_count) slot
)
INSERT INTO invitations (project_id,sender_id,recipient_id,status,created_at,responded_at,read_at)
SELECT project_id,sender_id,recipient_id,status,created_at,
 CASE WHEN status<>'pending' THEN created_at+interval '8 hours' END,
 CASE WHEN status IN ('accepted','rejected') OR (status='pending' AND slot%2=0) THEN created_at+interval '4 hours' END
FROM invitation_data ORDER BY number,slot;
