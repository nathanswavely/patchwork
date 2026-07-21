-- 023_credential_flags.sql
-- Persist the WebAuthn credential record fields we were silently dropping.
--
-- Issue #50: enrollment stored only 5 columns, so cred.Flags never survived.
-- Every credential loaded back from the database had BackupEligible = false,
-- the Go zero value. go-webauthn hard-compares that stored flag against the
-- assertion's BE bit and rejects on mismatch, so any synced passkey (iCloud
-- Keychain, 1Password, hybrid/QR — all BE=1) could enroll and then never log
-- in again. Platform authenticators that report BE=0 worked only by accident.
--
-- The columns are NULLABLE on purpose. NULL means "enrolled before we tracked
-- flags", which is honest — we cannot know what the authenticator reported.
-- Those rows keep loading as false and will fail once for a synced passkey;
-- the person re-enrolls. Deleting them instead would lock passkey-only people
-- out of every existing deployment, and Patchwork supports running with no
-- SMTP, so for some of them there is no other way back in.
ALTER TABLE credentials ADD COLUMN backup_eligible INTEGER;
ALTER TABLE credentials ADD COLUMN backup_state INTEGER;

-- Transport hints ("internal", "hybrid", "usb"…) as a JSON array. Advisory
-- only: browsers use them to suggest the right authenticator at login.
ALTER TABLE credentials ADD COLUMN transports TEXT;
