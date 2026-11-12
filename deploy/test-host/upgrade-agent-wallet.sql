-- Agent ledger actors and cash wallet ownership share the existing wallet owner scope.
ALTER TABLE point_ledger_entries DROP CONSTRAINT ledger_actor_type;
ALTER TABLE point_ledger_entries ADD CONSTRAINT ledger_actor_type CHECK(actor_type=ANY(ARRAY['user'::text,'admin'::text,'agent'::text,'system'::text]));
ALTER TABLE cash_destinations DROP CONSTRAINT cash_destinations_brand_id_member_id_fkey;
ALTER TABLE cash_destinations ADD CONSTRAINT cash_destinations_brand_id_member_id_fkey FOREIGN KEY(brand_id,member_id) REFERENCES wallet_owners(brand_id,id);
ALTER TABLE cash_quota_entries DROP CONSTRAINT cash_quota_entries_brand_id_member_id_fkey;
ALTER TABLE cash_quota_entries ADD CONSTRAINT cash_quota_entries_brand_id_member_id_fkey FOREIGN KEY(brand_id,member_id) REFERENCES wallet_owners(brand_id,id);
