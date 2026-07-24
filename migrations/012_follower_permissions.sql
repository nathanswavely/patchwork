-- Per-patch follower visibility settings.
-- Controls what followers can see. Default: all visible (transparent by default).
-- Format: {"events":true,"proposals":true,"charters":true,"members":true}
ALTER TABLE nodes ADD COLUMN follower_permissions TEXT DEFAULT '{"events":true,"proposals":true,"charters":true,"members":true}';
