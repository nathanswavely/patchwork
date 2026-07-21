# ADR 014: Quilt identity lives in the database; the icon is the bounded media exception

Date: 2026-07-19. Status: accepted.

## Context

Issues #12 and #13 ask for a Quilt settings tab in the admin panel (rename,
backups, danger zone, "eventually branding") and a custom icon that
represents the whole quilt in the quilt switcher and Connected Quilts.

Two documented decisions push back:

- Instance identity today is `patchwork.yaml` (`instance.name`,
  `instance.description`), loaded at startup. Renaming means shell access
  and a restart — an ops act, but renaming a community is a community act.
- ADR 007 (proposed) says the binary never touches media bytes: media
  goes through presigned bucket uploads, SQLite holds references only.
  But an instance icon must exist before any bucket is configured, the
  provider interface is not implemented yet, and non-technical admins
  cannot host a file elsewhere and paste a URL.

## Decision

**Split configuration by who changes it.** `patchwork.yaml` keeps
deployment concerns — domain, ports, SMTP, federation, database path,
modules, geography — owned by whoever operates the machine. Community
identity — name, description, icon — becomes database state
(`instance_settings` key/value table), editable by the instance admin in
the UI. A DB value overrides the yaml value; yaml remains the bootstrap
default and `instance.name` stays required. The settings page says when an
override is in effect so yaml edits don't silently lose. The domain is
never renameable from the UI or the DB: ActivityPub IDs minted with it are
permanent (config.go already warns about exactly this).

Startup-bound consumers (WebAuthn RPDisplayName, the email channel's
instance name) pick up a rename on the next restart; request-time
consumers (`GET /api/v1/instance`, SEO tags, the export's instance.json)
reflect it immediately.

**The icon is a bounded exception to ADR 007, not a repeal.** ADR 007's
reasons are unbounded storage, bandwidth, and transcode cost on the Pi,
and per-patch cost attribution. None apply to exactly one instance-owned
object: a single row (`instance_icon`, id=1) holding at most 512KB. The
bounds are the decision:

- One object per instance, ever. This is not a media pipeline and must
  not become one — patch media still waits for ADR 007's provider.
- PNG or JPEG only, square, 64–1024 px, ≤512KB, validated with
  `image.DecodeConfig` (header parse — the Pi never decodes or resizes
  pixels). No SVG uploads: same-origin-served SVG is a stored-XSS vector.
- Served at `GET /api/v1/instance/icon` with ETag + short max-age; the
  existing multi-quilt CORS middleware covers it, and `<img>` tags need no
  CORS anyway, so icons render across quilts.

**Defaults reuse the block vocabulary (ADR 004).** With no upload, the
icon endpoint serves a server-generated SVG quilt block (Pinwheel, Ohio
Star, Log Cabin, Nine Patch, Flying Geese, Churn Dash), tinted with the
branding color. The admin may pick one; unset means hash-assigned from
the quilt's name — stable but not chosen, the same rule tiles follow.
Embedded in the binary, so every instance has an icon from first boot.

**"Delete quilt" means wiping community data, not the deployment.** The
danger zone's wipe (`POST /api/v1/admin/wipe`, type-the-exact-name
confirmation, export-first nudge) deletes every row from every data table
in one transaction on a dedicated connection (`PRAGMA foreign_keys=OFF`
is per-connection and set outside the transaction; the pool keeps FKs on),
then deletes governance repos and re-initializes the instance repo. The
deployment — domain, yaml, container, TLS — survives. Sessions die with
the wipe, and the first account created afterwards becomes instance admin
again (existing bootstrap rule): a factory reset to first-run. The action
is recorded in the server log before deletion because the audit log is
itself wiped. Federation caveat: remote followers of wiped actors are left
dangling, as with any deletion.

**Seamrip boundary (extends ADR 002):** `instance_settings` and
`instance_icon` do not travel. The existing rule already says it —
community data travels; instance identity does not. A fork re-brands.

## Considered options

- Rename via yaml only: rejected — conflates the ops role with the
  community role; issue #12 exists because the admin UI is where the
  community acts.
- Icon as `branding.logo_url` reference (already in yaml): rejected as
  the mechanism — requires external hosting, exactly what grassroots
  admins don't have. `logo_url` survives untouched as a separate slot.
- Icon through ADR 007's presign flow: rejected — circular dependency
  (icon needed before any bucket exists) and the provider interface is
  unbuilt.
- SVG uploads: rejected — stored XSS on the instance origin; the trusted
  default SVGs are embedded in the binary, not user input.
- Stub the danger zone (UI only): rejected — a wipe within one
  transaction is implementable and testable; shipping a dead button
  teaches admins the danger zone is decorative.
- "Backups" as the tab section name: rejected — CONTEXT.md already pins
  seamrip ≠ backup. The section is "Data Export (Seamrip)"; real backups
  of the deployment are the data directory, an ops practice.
