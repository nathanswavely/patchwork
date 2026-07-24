-- Proposal state machine: draft → voting → approved/rejected → in_effect
-- Replaces the simpler 'status' field with a richer state model.

-- Add state column (new lifecycle states).
ALTER TABLE proposals ADD COLUMN state TEXT NOT NULL DEFAULT 'voting';
-- state values: 'draft', 'discussion', 'voting', 'approved', 'rejected', 'in_effect', 'withdrawn'

-- Store the base document SHA at proposal creation time for conflict detection.
ALTER TABLE proposals ADD COLUMN base_sha TEXT NOT NULL DEFAULT '';

-- Migrate existing status values to state.
UPDATE proposals SET state = 'in_effect' WHERE status = 'passed' OR status = 'approved';
UPDATE proposals SET state = 'rejected' WHERE status = 'rejected';
UPDATE proposals SET state = 'voting' WHERE status = 'open';
UPDATE proposals SET state = 'withdrawn' WHERE status = 'withdrawn';

-- Track when a proposal was made official (for manual-merge templates).
ALTER TABLE proposals ADD COLUMN applied_at TEXT;
ALTER TABLE proposals ADD COLUMN applied_by TEXT REFERENCES users(id);
