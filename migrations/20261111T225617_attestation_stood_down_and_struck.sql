-- Who a recorded leadership decision removed, and a record struck through as
-- entered in error
-- (docs/adr/2026-11-11-a-record-can-be-struck-through-and-names-who-stood-down.md).
--
-- `effect` says which side of the decision a name is on. Every name recorded
-- before this column was a name the meeting put in place, so 'seated' is the
-- default and nothing already written changes meaning. Only 'seated' names
-- carry a role: a 'stood_down' name is the record saying who came off, which
-- the effect already brings about by leaving them out of the council.
ALTER TABLE attestation_names ADD COLUMN effect TEXT NOT NULL DEFAULT 'seated'
    CHECK (effect IN ('seated', 'stood_down'));

-- A struck record stays in the table and on the Record, crossed out, and
-- counts for nothing: it is never the record in force and never the thing a
-- correction corrects. Nothing is deleted, the way a minutes book is kept.
-- `struck_by` names the admin who crossed it out, the initials beside the
-- line.
ALTER TABLE attestations ADD COLUMN struck_at TEXT;
ALTER TABLE attestations ADD COLUMN struck_by TEXT REFERENCES users(id);
