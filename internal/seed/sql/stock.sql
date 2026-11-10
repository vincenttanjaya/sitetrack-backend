WITH company AS MATERIALIZED (
 SELECT id FROM stores WHERE project_id IS NULL
), initial_supply AS (
 SELECT l.material_id,sum(l.quantity)::bigint AS quantity
 FROM deliveries d JOIN delivery_lines l ON l.delivery_id=d.id
 WHERE d.direction='company_to_project' GROUP BY l.material_id
), flows AS MATERIALIZED (
 SELECT CASE WHEN side.company THEN c.id ELSE project.id END AS store_id,l.material_id,
  sum(CASE WHEN d.status IN ('rejected','cancelled') THEN 0
       WHEN side.company=(d.direction='company_to_project') THEN -l.quantity ELSE l.quantity END)::bigint AS net,
  COALESCE(sum(l.quantity) FILTER (WHERE d.status='in_transit'),0)::bigint AS outbound,
  max(CASE WHEN side.company=(d.direction='company_to_project') THEN d.sent_at ELSE d.resolved_at END) AS last_delivery
 FROM deliveries d JOIN delivery_lines l ON l.delivery_id=d.id
 JOIN stores project ON project.project_id=d.project_id CROSS JOIN company c
 CROSS JOIN (VALUES (true),(false)) side(company)
 WHERE d.status='delivered' OR side.company=(d.direction='company_to_project')
 GROUP BY 1,2
), company_stock AS (
 SELECT c.id AS store_id,m.id AS material_id,
  COALESCE(initial_supply.quantity,0)+COALESCE(f.net,0)
   + CASE WHEN m.id%10=0 THEN 0 ELSE (m.id%40+5)*1000 END AS quantity,
  COALESCE(f.outbound,0) AS outbound,f.last_delivery
 FROM company c CROSS JOIN materials m
 LEFT JOIN initial_supply ON initial_supply.material_id=m.id
 LEFT JOIN flows f ON f.store_id=c.id AND f.material_id=m.id
)
INSERT INTO stock (store_id,material_id,quantity,reorder_point,outbound_quantity,last_delivery_at)
SELECT store_id,material_id,quantity,CASE WHEN material_id%3=0 THEN quantity+10000 ELSE 0 END,outbound,last_delivery
FROM company_stock
UNION ALL
SELECT f.store_id,f.material_id,f.net,0,f.outbound,f.last_delivery
FROM flows f CROSS JOIN company c WHERE f.store_id<>c.id;
