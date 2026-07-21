-- 004_membership_policy.sql
-- Add membership_policy to nodes and status to memberships.

ALTER TABLE nodes ADD COLUMN membership_policy TEXT NOT NULL DEFAULT 'open' CHECK (membership_policy IN ('open', 'approval_required', 'invite_only'));
ALTER TABLE memberships ADD COLUMN status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'pending', 'left'));
