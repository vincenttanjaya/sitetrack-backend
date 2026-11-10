WITH line_keys AS (
 SELECT d.id AS delivery_id,d.direction,
   1+(p.number*89+(((d.id-1)%(c.delivery_count/c.project_count))/4)*53+slot-1)%(c.material_count*19/20) AS material_id,
   CASE WHEN d.direction='project_to_company' THEN 1000+(p.number*13+d.id*3+slot*101)%7000
        ELSE 20000+(p.number*11+d.id*37+slot*53)%55000
   END AS quantity
 FROM deliveries d JOIN projects p ON p.id=d.project_id CROSS JOIN seed_config c
 CROSS JOIN LATERAL generate_series(1,c.line_count/c.delivery_count+
    CASE WHEN d.id%2=1 THEN 1 ELSE 0 END) slot
)
INSERT INTO delivery_lines (delivery_id,material_id,quantity,material_name,material_category,material_unit)
SELECT keys.delivery_id,keys.material_id,keys.quantity,m.name,m.category,m.unit
FROM line_keys keys JOIN materials m ON m.id=keys.material_id
ORDER BY keys.delivery_id,keys.material_id;
