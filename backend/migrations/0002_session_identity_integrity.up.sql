-- A session must reference the same global identity as its brand membership.
-- Existing migrations are immutable; additional constraints use a new file.
ALTER TABLE brand_members ADD CONSTRAINT members_identity_scope
 UNIQUE(brand_id,id,global_user_id);
ALTER TABLE sessions ADD CONSTRAINT session_member_identity
 FOREIGN KEY(brand_id,member_id,user_id)
 REFERENCES brand_members(brand_id,id,global_user_id);
