# ADR 013: Usernames are chosen at creation, never derived from email

Date: 2026-07-19. Status: accepted.

## Context

Magic-link signup created the account the moment the emailed link was
clicked, deriving the username from the email's local part (issue #9).
The person never chose their own handle, and their email prefix leaked
onto every public surface: the profile URL (`/users/{username}`), the
WebFinger `acct:` handle, and the ActivityPub actor's
`preferredUsername` — with federation live, that handle is visible to
other fediverse servers. Invite-link signup already let people choose,
but accepted any string and surfaced raw UNIQUE-constraint errors on
collision.

Two designs were on the table for the magic-link path:

1. **Provisional account, rename at onboarding** — create the user
   immediately (random or email-derived name), force a pick later. No
   new table, no second phase.
2. **Deferred creation** — clicking the link proves email control but
   creates no user; a short-lived signup token carries the proven email
   to a username-selection step, and the account is created only when
   its permanent name exists.

## Decision

**Deferred creation (option 2).** A provisional account instantly gets
an `ap_id`, an RSA keypair, and a WebFinger-resolvable actor; renaming
it later means either mutable federated identity (remote followers
break) or tombstoned throwaway actors. Creating the account only when
its permanent name exists preserves a single clean invariant:
**a username is immutable AP identity, fixed at birth.**

Mechanics:

- `VerifyMagicLink` for an unknown email consumes the magic link (all
  tokens stay single-use) and issues a **signup token**: 256-bit random,
  stored as a SHA-256 hash like magic and invite tokens, valid 60
  minutes, single-use. Browsers are redirected to
  `/signup/complete?token=…`; API clients get
  `{"status":"username_required","signup_token":…}`. Known emails log
  in exactly as before.
- `POST /api/v1/auth/signup` `{token, username, display_name}` creates
  the user inside one transaction: first-account admin bootstrap,
  `ap_id`, keypair, session — identical guarantees to the invite path.
  A duplicate email (two concurrent signup tokens) fails softly:
  "an account with this email already exists".
- If the signup token expires or is lost, the recovery door is the one
  the person came in through: request a new sign-in link.
- Signup tokens are ephemeral auth state and join sessions and
  magic/invite links outside the seamrip boundary (docs/adr/002).

**Username rules** (shared by invite and magic-link paths, enforced
server-side in `internal/auth/username.go`):

- Normalized (trimmed, lowercased), then validated:
  `^[a-z0-9][a-z0-9-]{1,28}[a-z0-9]$` — 3–30 chars, lowercase
  letters/digits/hyphens, alphanumeric at both ends. Same charset as
  patch slugs, because usernames and patch slugs share one WebFinger
  `acct:` namespace (users win on collision). A pleasant corollary:
  the `_system` sentinel is unspellable and can never be claimed.
- Availability is case-insensitive against existing usernames **and**
  rejects existing patch slugs, closing the AP-handle-squatting vector
  where a new user shadows a venue's federated actor.
- A reserved-word list blocks instance-authority words (`admin`,
  `moderator`, `patchwork`, …) and API/SPA namespace words (`api`,
  `me`, `settings`, …) — `acct:admin@…` federating as a random member
  is an impersonation surface.
- **Immutable after creation.** No rename endpoint exists.

**Existing users are grandfathered.** Email-derived usernames created
before this decision keep working (profile URLs, exact-match WebFinger);
validation gates new names only. Case-insensitive uniqueness prevents
registering the lowercase twin of a grandfathered name.

## Consequences

- Magic-link signup becomes two steps (click link → choose username).
  That friction is the feature: the handle is chosen, not assigned.
- Abandoning the username step orphans nothing — no user row, no
  keypair, no actor; the token hash expires silently.
- One more table (`signup_tokens`, migration 020) and one more SPA
  page (`/signup/complete`).
- Renaming (e.g. for grandfathered email-prefix usernames) is
  deliberately unsolved here; if it ever lands it must answer the AP
  identity question (Move activity? alias?) in its own ADR.
