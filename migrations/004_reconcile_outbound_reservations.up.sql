-- Drain project/store changes, then block deliveries and stock writers until
-- the rebuilt reservations commit. Deliveries are written before stock by the API.
LOCK TABLE projects IN SHARE MODE;
LOCK TABLE deliveries IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE stores IN SHARE MODE;
LOCK TABLE delivery_lines, stock IN SHARE ROW EXCLUSIVE MODE;

-- SUM(bigint) returns numeric so even totals that overflow bigint can be diagnosed.
CREATE TEMP TABLE migration_outbound_reserves ON COMMIT DROP AS
SELECT CASE WHEN d.direction = 'company_to_project'
            THEN (SELECT id FROM stores WHERE project_id IS NULL)
            ELSE s.id END AS store_id,
       l.material_id, sum(l.quantity) AS outbound_quantity,
       max(d.sent_at) AS last_dispatch_at, min(d.id) AS sample_delivery_id
FROM deliveries d
JOIN delivery_lines l ON l.delivery_id = d.id
LEFT JOIN stores s ON s.project_id = d.project_id AND d.direction = 'project_to_company'
WHERE d.status = 'in_transit'
GROUP BY 1, 2;

DO $$
DECLARE
    invalid record;
BEGIN
    SELECT sample_delivery_id INTO invalid
    FROM pg_temp.migration_outbound_reserves
    WHERE store_id IS NULL
    LIMIT 1;

    IF FOUND THEN
        RAISE EXCEPTION 'cannot reconcile outbound reservations: in-transit delivery % has no sender store', invalid.sample_delivery_id;
    END IF;

    SELECT r.store_id, r.material_id, COALESCE(s.quantity, 0) AS on_hand, r.outbound_quantity
    INTO invalid
    FROM pg_temp.migration_outbound_reserves r
    LEFT JOIN stock s USING (store_id, material_id)
    WHERE COALESCE(s.quantity, 0) + r.outbound_quantity > 9223372036854775807
    ORDER BY r.store_id, r.material_id
    LIMIT 1;

    IF FOUND THEN
        RAISE EXCEPTION 'cannot reconcile outbound reservations: refund capacity exceeded for store %, material % (on hand %, reserved %)',
            invalid.store_id, invalid.material_id, invalid.on_hand, invalid.outbound_quantity;
    END IF;
END $$;

UPDATE stock s
SET outbound_quantity = r.outbound_quantity::bigint
FROM pg_temp.migration_outbound_reserves r
WHERE s.store_id = r.store_id AND s.material_id = r.material_id
  AND s.outbound_quantity <> r.outbound_quantity;

UPDATE stock s
SET outbound_quantity = 0
WHERE s.outbound_quantity <> 0 AND NOT EXISTS (
    SELECT 1 FROM pg_temp.migration_outbound_reserves r
    WHERE r.store_id = s.store_id AND r.material_id = s.material_id
);

-- A legacy sender may have no row after dispatching its entire balance.
INSERT INTO stock (store_id, material_id, outbound_quantity, last_delivery_at)
SELECT r.store_id, r.material_id, r.outbound_quantity::bigint, r.last_dispatch_at
FROM pg_temp.migration_outbound_reserves r
WHERE NOT EXISTS (
    SELECT 1 FROM stock s WHERE s.store_id = r.store_id AND s.material_id = r.material_id
);
