ALTER TABLE stock
    ADD COLUMN outbound_quantity bigint NOT NULL DEFAULT 0 CHECK (outbound_quantity >= 0),
    ADD CONSTRAINT stock_refund_headroom CHECK (quantity <= 9223372036854775807 - outbound_quantity);

ALTER TABLE delivery_lines
    ADD COLUMN material_name text,
    ADD COLUMN material_category text,
    ADD COLUMN material_unit text;

UPDATE delivery_lines l
SET material_name = m.name, material_category = m.category, material_unit = m.unit
FROM materials m
WHERE m.id = l.material_id;

ALTER TABLE delivery_lines
    ALTER COLUMN material_name SET NOT NULL,
    ALTER COLUMN material_category SET NOT NULL,
    ALTER COLUMN material_unit SET NOT NULL;
