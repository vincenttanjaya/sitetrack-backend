INSERT INTO users (id,name,email,password_hash,role,created_at)
SELECT n,
  CASE n WHEN 1 THEN 'Amira Rahman' WHEN 2 THEN 'Rashid Hassan'
         WHEN 3 THEN 'Priya Menon' WHEN 4 THEN 'Musa Ibrahim'
         ELSE (ARRAY['Amina','Daniel','Hassan','Leila','Omar','Priya','Sara','Yusuf','Nora','Khalid'])[(n % 10)+1]
          || ' ' || (ARRAY['Haddad','Okafor','Nair','Aziz','Santos','Tan','Mensah','Khan','Issa','Fernandes'])[(n/10 % 10)+1]
  END,
  CASE n WHEN 1 THEN 'site-admin@sitetrack.test' WHEN 2 THEN 'warehouse@sitetrack.test'
         WHEN 3 THEN 'member@sitetrack.test' WHEN 4 THEN 'invitee@sitetrack.test'
         ELSE 'crew-' || n || '@sitetrack.test'
  END,
  c.password_hash,
  CASE WHEN n=1 THEN 'superadmin' WHEN n=2 OR n%269=0 THEN 'company_store_manager' ELSE 'user' END,
  (c.anchor::timestamp AT TIME ZONE 'UTC') - interval '6 years' + (n % 730)*interval '1 day'
FROM seed_config c CROSS JOIN generate_series(1,c.user_count) n;
