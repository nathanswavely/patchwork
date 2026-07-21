# ADR 007: Media — the instance stores references, patches own their bytes

Date: 2026-07-13. Status: proposed.

## Context

Patchwork abstains from media (images, video, even avatars-as-uploads) to
hold the potato constraint: the binary must run on a Raspberry Pi 4, and
storing, serving, and transcoding media is exactly what a Pi cannot do.
But communities want flyers, show photos, and eventually video, and the
costs of hosting them should land on the patch that incurs them — visibly,
on their own bill — not silently on whoever volunteers the Pi.

## Decision

The binary never touches media bytes. A provider interface in
`internal/media` (`PresignUpload`, `PresignRead`, `Delete`, `Usage`) hands
the browser a presigned URL for a direct-to-bucket upload (S3 SigV4 is a
signature computation, implementable on stdlib crypto — no AWS SDK).
SQLite stores only a reference row per object: node, object key, mime,
size, uploader, and **required alt text** (a11y baseline, and the graceful
remnant when a bucket dies). Serving is a plain URL to the bucket; the
patch's provider bears storage and bandwidth. Zero bytes, zero egress
through the instance.

Two provider implementations, one abstraction:

- **Instance-pooled (default):** the instance admin configures one
  S3-compatible account (reference target: Cloudflare R2 — zero egress
  fees) once; patches draw on it with per-patch metering via `Usage`.
  This is the path for non-technical patch admins; how usage is paid for
  is ADR 008's problem, not this package's.
- **BYO bucket (per-patch):** a patch plugs in its own bucket and scoped
  key. Power-user path and portability valve — the patch's media outlives
  the instance because it was never the instance's to begin with.

Video is the same pattern one step further out: embed references to
providers that do their own transcode and delivery (patch pays the
provider directly). The binary never transcodes anything. Images are
resized client-side before upload; the server enforces size caps at
presign time, since there is nothing behind it to downscale.

**Moderation:** the instance embeds content it does not host, so the
instance admin gets a hard **delist** — purging the reference row (and
issuing `Delete` where the provider allows) even though the bytes live
elsewhere. Without this the antifascist baseline is unenforceable against
media.

**Credentials:** BYO keys are a honeypot in the instance DB. They are
encrypted at rest with an instance key, and the setup flow insists on
keys scoped to a single bucket.

**Seamrip boundary (extends ADR 002):** reference rows travel; provider
credentials do not. BYO patches carry their media automatically (the
bucket is theirs). Pooled objects belong to the instance's account — a
fork re-homes them or accepts the loss; the export README says so.

**Federation:** AP attachments carry URLs; remote instances fetch from
the bucket directly. Nothing new federates.

## Considered options

- Store bytes on the instance disk: rejected — breaks the potato
  constraint on disk, bandwidth, and backup size, and centralizes costs
  on the host, which is the exact failure this ADR exists to avoid.
- Proxy media through the binary (upload and serve): rejected — every
  image view becomes Pi bandwidth; the presigned flow costs one signature
  instead.
- Server-side thumbnails/transcoding: rejected — CPU the Pi doesn't
  have; client-side resize plus size caps covers images, embed providers
  cover video.
- Hard-code one CDN/provider: rejected — the interface is the point;
  providers die (see ADR 008's fiscal-host lesson) and instances differ.
- UI coinage for media: deliberately none, following the "Person"
  precedent — the textile vocabulary is for community structures, not
  artifacts. Photos are photos.
