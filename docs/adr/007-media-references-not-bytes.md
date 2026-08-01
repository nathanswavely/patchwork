# ADR 007: Media — the instance stores references, patches own their bytes

Date: 2026-07-13. Status: **partly implemented** — the reference half ships
(migration 052); the presigned upload half does not. See the amendment at
the end.

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

## Amendment, 2026-08-01: the reference half, without the bucket

Everything above still stands. What shipped is the half of it that needs
no infrastructure: `nodes.image_url` / `image_alt` and the same pair on
`events`, set by pasting a URL to something the patch already hosts.

The reason for splitting it this way is that the decision above has a
dependency the instance operator has to satisfy before any of it is
useful — an S3-compatible account, configured, paid for. Meanwhile an
arts-scene instance had no flyers and no show photos at all, and
`users.avatar_url` had been holding a remote reference since migration
001. The model was already here. Only the convenience was missing.

When the presigned flow lands it becomes a way to *produce* one of these
URLs. Same column, same reference row, same delist. Nothing to migrate.

**What the reference half gets right on its own:**

- The binary still never touches bytes. It never fetches the image
  either: the browser does, straight from wherever the patch keeps it.
- Alt text is required whenever a URL is set, which is the a11y baseline
  and the graceful remnant. It matters more here than it would with a
  bucket we control, because these references die more often.
- `https` only. A patch page served over TLS that pulls an image over
  plain http gets it blocked as mixed content, so an http URL is a
  picture nobody will ever see. Refused at the form rather than blank on
  every visitor's screen.
- A `data:` URI is refused, which is the same rule as the ADR's first
  line: it would put the bytes in SQLite.
- **Delist works**, as `remove_image` on a report. It clears the
  reference and leaves the patch and the event alone — proportionate in
  the way `reset_appearance` is, and the antifascist baseline is
  unenforceable against media without it.
- Reference rows travel in a seamrip, and for a pasted URL the "fork
  re-homes them or accepts the loss" caveat does not apply at all: the
  bytes were never the instance's.

**What it gets wrong, or at least differently:**

- **An arbitrary URL is not a bucket the patch controls.** Rendering it
  tells that host every viewer's IP, and a tracking pixel is
  indistinguishable from a flyer. This is already true of
  `users.avatar_url`, so it is consistent rather than new, but the ADR
  above never had to think about it because a presigned bucket URL points
  somewhere the patch chose deliberately. Worth revisiting if the
  reference model outlives the upload one.
- **No size cap is enforceable.** The ADR caps at presign time. Here
  there is no presign, and a patch can link a 20MB photograph. The cost
  lands on visitors' bandwidth rather than the instance's, which is the
  right party by this ADR's own argument, but it is not a cap.
- **Federation is untouched.** AP attachments were named above as
  carrying URLs; nothing here adds them. A remote instance sees no image.
