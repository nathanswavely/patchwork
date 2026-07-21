# Security Policy

Patchwork instances hold real community data: names, emails, memberships
(including ones people chose to hide from public view). Vulnerability
reports are taken seriously.

## Reporting a vulnerability

**Do not open a public issue.** Report privately, either via GitHub's
"Report a vulnerability" (Security tab) or by email to
<nathan@swavelycreative.com>.

Include what you can: affected endpoint or component, reproduction steps,
and impact as you understand it. You'll get an acknowledgment within a few
days. Patchwork has a single volunteer maintainer, so fixes are
best-effort — but data-exposure and auth issues jump the queue.

## Supported versions

The latest release (and `main`). Patchwork is self-hosted; operators are
expected to pull updates. If you run an instance, watch the releases feed.

## Out of scope

- Denial of service against your own self-hosted instance
- Reports requiring a compromised instance admin account (the admin
  already holds the data by design — see `docs/adr/002` on what an
  export contains)
- Vulnerabilities in dependencies with no demonstrated impact on
  Patchwork (report those upstream, though a heads-up is appreciated)
