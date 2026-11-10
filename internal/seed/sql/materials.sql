INSERT INTO materials (id,name,category,unit)
SELECT n,
 (ARRAY['OPC cement','Washed sand','Crushed aggregate','Rebar','Plywood','Drainage pipe','Sealant','Conduit'])[(n%8)+1]
  || ' / grade ' || ((n*7)%18+1) || ' / SKU ' || n,
 (ARRAY['Cement','Aggregates','Aggregates','Steel','Formwork','Plumbing','Finishes','Electrical'])[(n%8)+1],
 (ARRAY['bag','tonne','tonne','bar','sheet','metre','cartridge','metre'])[(n%8)+1]
FROM seed_config c CROSS JOIN generate_series(1,c.material_count) n;
