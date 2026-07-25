-- Migration 041: complete the leadership half of governance_config.
--
-- Migration 013's column default for nodes.governance_config carries only
-- the decision-making keys; the leadership keys (leadership_model,
-- succession_method, max_admins, inactivity_days) were only ever written
-- by governance.SyncRulesToDB — a path most patches never trigger — so
-- nearly every patch rendered "Leadership: Not set". SQLite can't change
-- an existing column's default; the creation paths now write the config
-- explicitly (governance.SyncConfigToDB), and this backfills the rows
-- already dealt.
--
-- A missing leadership_model is the reliable "never synced" marker: sync
-- always fills it (defaults backstop the git rules), so a row that has it
-- was synced and must not be touched — its other leadership keys may be
-- legitimately absent (omitempty zeros, e.g. a minimal-template patch's
-- inactivity_days).

-- Rows with no usable config at all get the full default
-- (mirrors governance.DefaultRules).
UPDATE nodes
SET governance_config = '{"decision_method":"majority","quorum_percent":0,"default_vote_duration_hours":72,"amendment_threshold":"majority","amendment_auto_apply":true,"succession_policy":"longest_tenure","min_voting_tenure_days":0,"leadership_model":"maintainer","succession_method":"admin_nominate","max_admins":3,"inactivity_days":90}'
WHERE governance_config IS NULL OR json_valid(governance_config) = 0;

-- Never-synced rows keep their decision keys and gain the default
-- leadership block (admin_term_months 0 stays absent, matching what
-- SyncRulesToDB's omitempty marshaling produces).
UPDATE nodes
SET governance_config = json_set(governance_config,
	'$.leadership_model', 'maintainer',
	'$.succession_method', 'admin_nominate',
	'$.max_admins', 3,
	'$.inactivity_days', 90)
WHERE json_type(governance_config, '$.leadership_model') IS NULL;
