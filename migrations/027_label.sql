-- The Label: a quilt's stewardship disclosure (docs/adr/023).
--
-- One row, like instance_icon: the Label is a statement about the
-- deployment, and there is exactly one deployment. Costs are structured
-- line items; everything else is prose in the stewards' own voice.
-- None of these tables enter the seamrip boundary (internal/seamrip) —
-- a fork's Label would be false on arrival, so a fork writes its own.

CREATE TABLE label (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    -- Markdown, the stewards' own words. The warmth lives here, not in
    -- form fields.
    prose TEXT NOT NULL DEFAULT '',
    -- Optional: how to support the work and how to reach the stewards.
    support_url TEXT NOT NULL DEFAULT '',
    feedback_url TEXT NOT NULL DEFAULT '',
    -- One instance-wide currency for all cost items (ISO 4217 code).
    currency TEXT NOT NULL DEFAULT 'USD',
    -- The removable provenance line prefilled by a seamrip import: the
    -- new home looking back. Empty means removed or never a fork.
    seamripped_from_name TEXT NOT NULL DEFAULT '',
    seamripped_from_url TEXT NOT NULL DEFAULT '',
    -- A Label cannot publish with zero listed stewards; the handler
    -- enforces the floor on publish and re-checks when stewards change.
    published INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Stewards: people publicly accountable for how the quilt is run.
-- Deliberately NOT a view of users.role — holding the admin bit never
-- publishes a person (docs/adr/023). Each row is one person's listing,
-- and the listing is theirs: an instance admin may add themselves as
-- listed, but adding anyone else creates an unlisted invitation that
-- only that person can accept (listed=1) — consent to appear is given
-- by the person appearing, in the spirit of ADR 006.
CREATE TABLE label_stewards (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    -- Their own words for what they look after ("keeps the lights on").
    blurb TEXT NOT NULL DEFAULT '',
    listed INTEGER NOT NULL DEFAULT 0,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

-- Structured cost line items. Amounts are integer minor units (cents),
-- never floats — same rule as ADR 008's ledger. A prose-only Label is
-- legal and simply has no rows here: no total, no staleness detection.
CREATE TABLE label_cost_items (
    id TEXT PRIMARY KEY,
    service TEXT NOT NULL,                  -- "Hetzner CX22"
    purpose TEXT NOT NULL DEFAULT '',       -- "the server"
    why TEXT NOT NULL DEFAULT '',           -- the values live here
    amount_minor INTEGER NOT NULL,
    period TEXT NOT NULL DEFAULT 'monthly' CHECK (period IN ('monthly', 'yearly')),
    -- The date this figure was stated (or last reviewed). The page
    -- surfaces decay past a threshold — self-reported numbers rot
    -- silently, and a stale money claim wears the costume of an audit.
    stated_on TEXT NOT NULL,
    -- Cost-source hook (ADR 008's provider pattern; docs/adr/023).
    -- 'manual' is first-class and the only source shipped. A future
    -- provider binds to a specific resource (source_binding), never an
    -- account, and stamps fetched_at on refresh.
    source TEXT NOT NULL DEFAULT 'manual',
    source_binding TEXT NOT NULL DEFAULT '',
    fetched_at TEXT NOT NULL DEFAULT '',
    position INTEGER NOT NULL DEFAULT 0
);
