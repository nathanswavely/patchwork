# Deploying Patchwork

This guide takes you from a fresh server to a running, claimed instance. The
[README](../README.md#quickstart) has the short version. This is the long
one.

## Prerequisites

- A server with Docker and Docker Compose (a Raspberry Pi 4 with 2GB RAM is
  plenty, and any small VPS works).
- A domain name with an A/AAAA record pointing at the server.
- Ports 80 and 443 reachable from the internet (Caddy needs both for
  automatic HTTPS via Let's Encrypt).

No email server is required. Magic links print to the server log without one,
and invite links plus passkeys never touch email at all.

## 1. Configure

```bash
git clone https://github.com/patchwork-toolkit/patchwork.git
cd patchwork
cp patchwork.yaml.example patchwork.yaml
```

Edit `patchwork.yaml`. The fields that matter most:

| Field | What it does |
|---|---|
| `instance.name` | Your community's name. Required. |
| `instance.domain` | Your public domain, e.g. `quilt.example.org`. Required for federation, and used in magic-link URLs and ActivityPub IDs. |
| `instance.description` | Shown on the instance endpoint and in discovery. |
| `geographic` | Map center (lat/lng) and radius in km. |
| `modules` | Toggle map, governance, ledger. |
| `submissions` | Community submissions of unclaimed patches: `enabled` gates the whole feature, `auto_approve` skips admin review. |
| `smtp` | Optional. Without it, magic links print to the log. Set `PATCHWORK_SMTP_PASS` in the environment to keep the password out of the file. The port picks the TLS mode: 465 (or 2465) connects with implicit TLS; any other port (587, 25, …) connects in plaintext and upgrades via STARTTLS when the server offers it. Auth uses PLAIN when advertised and falls back to LOGIN for servers that only offer that (Office 365, some appliance relays). See [Email](#email-optional) for provider recipes. |
| `multi_quilt` | When true, public GET endpoints send CORS headers so other Patchwork SPAs can merge your quilt into theirs. |
| `federation.enabled` | ActivityPub federation. **Read the federation section below before enabling.** |

Then tell Caddy your domain:

```bash
echo "DOMAIN=quilt.example.org" > .env
```

`docker compose` reads `.env` automatically and passes `DOMAIN` to the Caddy
container. With a real domain and ports 80/443 open, HTTPS certificates are
issued automatically on first request. If `DOMAIN` is unset, Caddy serves
plain HTTP on `localhost` (fine for trying it out locally).

## 2. Run

There are two ways to run Patchwork, and **you should pick one and stay on
it** — mixing them silently replaces a tested image with an untested one.

| | Build from source | Prebuilt image |
|---|---|---|
| Command | `docker compose up -d --build` | `docker compose pull && docker compose up -d` |
| Needs | ~2GB free RAM at build time | just a network pull |
| Image | compiled on this box | published by CI to ghcr |
| Setup | none (compose has `build: .`) | `docker-compose.override.yml` |

Build from source if you're self-hosting a fork or don't publish images.
Use the prebuilt image on small servers — building means `npm ci`, vite, and
a static CGO Go link on the same box that's serving the site, and on a 2GB
VPS with no swap that's a plausible OOM.

**Build from source:**

```bash
docker compose up -d --build
```

**Run a prebuilt image:**

```bash
cp docker-compose.override.yml.example docker-compose.override.yml
# edit the image tag if you publish somewhere other than the reference repo
docker compose pull
docker compose up -d
```

Compose merges `docker-compose.override.yml` automatically; its `image:`
takes precedence over the base file's `build: .`. Once that override exists,
**never pass `--build`** — compose would build locally and tag the result
with the ghcr name, replacing the CI-tested image in place with no warning.
That file is deployment state: keep it in your server's config backup, or it
disappears on a host rebuild and the build path silently becomes the default
again.

Either way, this builds or fetches the Svelte frontend, compiles it into the Go binary, and starts
two containers: `patchwork` (the app, distroless, non-root) and `caddy`
(reverse proxy + TLS). All app state — the SQLite database and governance
repos — lives in the `data` named volume, mounted at `/data` (the image's
working directory, so the example config's relative `database.path` resolves
inside it). The app refuses to start in a container if the database path is
not on a mounted volume, so a config mistake here is a visible startup error
instead of silent data loss on the next recreate.

Check it's up:

```bash
docker compose logs patchwork
curl -s https://quilt.example.org/api/v1/health
```

On a fresh database you'll see this in the log:

```
first run: no accounts exist yet — the first account created will become the instance admin
```

## 3. Claim your instance (first admin)

**The first account created on a fresh instance becomes the instance
admin.** Do this immediately after first boot, before sharing the URL:

1. Open `https://your-domain/login` and enter your email.
2. If SMTP is configured, click the link in your email. Otherwise grab the
   link from the log:
   ```bash
   docker compose logs patchwork | grep "auth/verify"
   ```
3. You're in, as admin. Go to Settings → Security and **enroll a passkey**
   so you can sign in without magic links from now on.

Every account after the first is a regular member.

## 4. Invite your community

As admin, generate invite links from the admin dashboard (or
`POST /api/v1/auth/invite-link`). Each link can be single- or multi-use, with
an optional expiry. Share them out-of-band: Signal, email, a QR code on a
flyer. Whoever clicks one creates an account and enrolls a passkey, no email
round-trip needed.

## Email (optional)

Start by asking whether you need it. Invite links and passkeys run a full
instance with no email service at all, and for a launch weekend that is the
right amount of infrastructure. What email adds, once the instance is real:
magic-link login for people who lost their passkey or never enrolled one, and
notification delivery to people who aren't checking the site. Set it up when
that starts mattering, not before.

### What the app actually needs

One thing: an **outbound SMTP relay**. Patchwork never receives mail — both
email paths (magic links, notifications) only *submit* messages. The relay
must offer:

- SMTP submission on **port 587 (STARTTLS) or 465 (implicit TLS)** — the
  port you configure picks the mode. Auth is PLAIN, with a LOGIN fallback
  for relays that only offer that.
- A From address on your domain that the relay accepts, which in practice
  means verifying the domain with the relay (an SPF record and a DKIM record
  they give you).

```yaml
smtp:
  host: "smtp.resend.com"
  port: 587
  user: "resend"
  from: "noreply@quilt.example.org"
  # pass via PATCHWORK_SMTP_PASS in the environment
```

Everything else — an inbox at `hello@your-domain`, replies to the From
address landing somewhere, DMARC reports — is for *humans*, not the app.
You want it (a community should be reachable, and some providers insist on a
working address during signup), but it's a mailbox-or-forwarding question you
can solve separately from the relay.

### Recipe A: Cloudflare DNS + Email Routing + Resend (free, recommended)

Two free accounts, and all your DNS records live in one editor.

1. Add your domain to [Cloudflare](https://dash.cloudflare.com) (free plan)
   and switch nameservers at your registrar. This is the one-time cost of
   this recipe, and it's what buys the rest: registrar DNS panels often
   couple the MX section to modal "email settings" where enabling one thing
   silently drops another. Cloudflare DNS is just records.
2. Enable **Email Routing** on the domain, verify a destination mailbox, and
   forward `hello@` (and a catch-all if you like) to it. The wizard adds its
   own MX and SPF records. Free tier covers 200 addresses — more than any
   community instance needs.
3. Create a [Resend](https://resend.com) account, add the domain, and create
   the records it asks for (SPF on a `send.` subdomain plus one DKIM record —
   they coexist fine with Email Routing's records). Generate an API key: host
   `smtp.resend.com`, port `587`, user `resend`, the key as password. Free
   tier is 3,000 emails/month, 100/day.
4. Add a DMARC record so receivers stop guessing:
   `_dmarc TXT "v=DMARC1; p=none; rua=mailto:hello@your-domain"`.

The relay slot is swappable: if Resend's free tier changes or you outgrow it,
any 587/STARTTLS relay (Postmark, Mailgun, SES once you're out of its
sandbox) drops into the same recipe — new records, new credentials in
`patchwork.yaml`, nothing else moves.

### Recipe B: one paid mailbox provider (simplest, ~$10–50/yr)

[Migadu](https://migadu.com), [Purelymail](https://purelymail.com), or
[Fastmail](https://fastmail.com): a single account gives you a real
`hello@your-domain` mailbox *and* an SMTP submission endpoint on 587, so
there's no forwarding service and no relay account. DNS stays wherever your
domain already is; you add the MX/SPF/DKIM records they list.

Two caveats. Cheap tiers cap outbound — Migadu's Micro plan allows on the
order of tens of messages per day, which notification fan-out on a busy
instance can blow through, so check the number against your community's size.
And mailbox providers' terms are written for human mail; an app submitting
notifications is usually fine at community scale, but it's their call, not
yours.

### Recipe C: registrar DNS + forwarding service + relay (what the reference instance runs)

lancasterpatchwork.org runs Namecheap DNS + [ImprovMX](https://improvmx.com)
free (inbound: root MX + SPF, `hello@` → admin mailbox) + Resend (outbound,
as in recipe A). It works and costs nothing, but it's three accounts, and
registrar DNS panels are the weak point: **Namecheap's Email Settings lock
the MX section to either-or modes**, so setting custom MX for ImprovMX means
flipping the whole domain to "Custom MX" mode and re-entering things the
other modes managed for you. If your domain is stuck at a registrar you
can't or won't move, this recipe is fine — just expect the DNS panel to
fight you once.

### A note on Amazon SES

Cheapest sending anywhere at scale and a fine relay *if you already live in
AWS* — it speaks 587/STARTTLS with SMTP credentials. As a first email setup
for a community admin it's the wrong tool: the account wants a credit card,
new accounts are sandboxed to verified recipients until a human-reviewed
support case approves production access, and its "inbound" delivers to an S3
bucket, not a mailbox — forwarding `hello@` means writing a Lambda. Skip it
unless AWS is already home.

### Verifying it works

Configure `smtp`, restart, then log out and request a magic link to an
address you control. If nothing arrives, the app log has the error:

```bash
docker compose logs patchwork | grep -i "magic link"
```

Then send yourself one from an external account to `hello@your-domain` to
confirm the inbound leg, and check the outbound message's headers (Gmail:
"Show original") for `SPF: PASS` and `DKIM: PASS` — failing those is the
difference between delivered and spam-foldered.

## Backups

### What to back up

Three things, and it is easy to notice only the first:

1. **`patchwork_data`** — the SQLite database (plus its `-wal` journal) and
   the governance git repos. The obvious one.
2. **`patchwork_caddy_data`** and **`patchwork_caddy_config`** — ACME account
   keys and issued TLS certificates. Losing these isn't fatal (Caddy
   re-issues) but re-issuance is rate-limited, and you'd be spending that
   budget during the incident where you're already restoring from backup.
3. **The host config directory** (`/opt/patchwork` or wherever your compose
   file lives) — `patchwork.yaml`, `docker-compose.override.yml`, `.env`,
   `Caddyfile`. These are gitignored or untracked by design, so a repo clone
   will *not* bring them back. Without them you can restore the data and
   still not reproduce the deployment. **`patchwork.yaml` and `.env` can hold
   SMTP credentials — keep this archive `600` and never commit it.**

**Copying just the `.db` file is the classic mistake.** It misses the WAL
journal (your most recent writes) and every governance repo.

### Taking a backup

The app container is distroless (no shell), so archive the volume from a
helper container:

```bash
docker run --rm -v patchwork_data:/data -v "$PWD/backups":/backup alpine \
  tar czf "/backup/patchwork-$(date +%F).tgz" -C /data .
```

Sanity-check it: the archive should contain `patchwork.db`
(`tar tzf backups/patchwork-*.tgz | grep patchwork.db`). An empty archive
means the volume isn't the one the app writes to. Make the check part of the
script rather than a thing you remember to do — and put the retention prune
*after* it, so a failed run can't rotate a good archive out.

**On consistency.** A `tar` of a live WAL database is not a guaranteed-
consistent snapshot: the `.db` and `-wal` files are copied at slightly
different moments, and a checkpoint landing between them can produce a
mismatched pair. Patchwork runs `wal_checkpoint(TRUNCATE)` at *startup*, and
`wal_autocheckpoint` fires on WAL *size* (every 1000 pages), not on a timer —
so on a quiet instance the WAL may sit uncheckpointed for a long time. Don't
assume a checkpoint has happened recently.

In practice a small instance restores fine (see the rehearsal below), but if
you want certainty, snapshot the database through SQLite instead of copying
its files, and tar the governance repos separately:

```bash
docker run --rm -v patchwork_data:/data:ro -v "$PWD/backups":/backup alpine sh -c \
  'apk add --no-cache sqlite >/dev/null && \
   rm -f /backup/snapshot.db && \
   sqlite3 /data/patchwork.db "VACUUM INTO '\''/backup/snapshot.db'\''"'
```

`VACUUM INTO` uses SQLite's own machinery, so the result is a coherent,
self-contained database (no `-wal` alongside it) no matter what was being
written at the time.

Two details that will bite you otherwise. Write the snapshot to a *mounted
backup directory*, not into `/data`: the helper container runs as root, so a
snapshot written into the volume would be root-owned inside a tree the app
owns as uid 65532. And `VACUUM INTO` refuses to overwrite an existing file,
so clear the target first — otherwise the first run succeeds and every run
after it fails.

### Off-site: a backup on the same disk is not a backup

If your archives live on the same filesystem as the volume they archive,
they protect you against fat-fingered deletes and bad migrations — real
problems — but against nothing that takes the machine. Disk failure, a host
rebuild, an account suspension, or a root compromise takes the live database
*and* every archive of it in one stroke, because they were never separate
things.

Push a copy somewhere else. The reference instance uses
[restic](https://restic.net) to Backblaze B2 — the whole dataset is a few
megabytes, so this costs effectively nothing:

```sh
# /root/.config/restic/b2.env  — chmod 600, never committed
AWS_ACCESS_KEY_ID=<application keyID>
AWS_SECRET_ACCESS_KEY=<applicationKey>
RESTIC_REPOSITORY=s3:https://s3.us-east-005.backblazeb2.com/your-bucket/patchwork
RESTIC_PASSWORD=<long random passphrase>
```

**Use B2's S3-compatible endpoint, not restic's native `b2:` backend.** The
native backend calls `b2_authorize_account` at API v1 (restic 0.16) or v3
(restic 0.19); Backblaze now accepts only v4 for application keys, so `b2:`
fails with `not currently supported on API version number N` — including on
current restic. The `s3:` backend sidesteps that entirely and reaches the
same objects, so a repository created either way is readable by the other.
Your account's endpoint hostname is region-specific; read it from
`b2_authorize_account` (v4) as `s3ApiUrl`, or from the bucket page in the
console.

Ubuntu's packaged restic is 0.16.4 and predates several B2 fixes. Install a
current release from GitHub, verify it against the published `SHA256SUMS`,
and **call it by absolute path from cron** — cron's `PATH` is `/usr/bin:/bin`,
so a bare `restic` silently picks the distro copy even when a newer one wins
in an interactive shell.

Two properties are worth setting up deliberately:

**Use a credential that cannot delete.** Create the B2 application key
scoped to the one bucket, with `listBuckets,listFiles,readFiles,writeFiles`
and *without* `deleteFiles`. The web console's coarse "Read and Write" option
includes delete; the restricted key has to come from the B2 CLI:

```bash
b2 key create --bucket your-bucket patchwork-backup-append \
  listBuckets,listFiles,readFiles,writeFiles
```

**Know exactly what this buys you, because it is not "deletes fail."**
Verified against a live B2 bucket: with `writeFiles` but no `deleteFiles`, a
delete through the S3 endpoint becomes a B2 *hide marker*. `restic forget`
reports success and the snapshot disappears from `restic snapshots` — but
the underlying file version is still in the bucket (given the keep-all-
versions lifecycle above) and can be recovered by clearing the hide marker
with a privileged key.

So the guarantee is: **an attacker with root can hide your backup history but
cannot destroy it.** That is the property worth having — ransomware and
malicious wipes are recoverable — but don't mistake a clean `restic forget`
for proof that the restriction isn't working. Verify capabilities directly:

```bash
curl -s -u "$KEY_ID:$APP_KEY" \
  https://api.backblazeb2.com/b2api/v4/b2_authorize_account
```

`apiInfo.storageApi.allowed` should list exactly
`writeFiles, listBuckets, listFiles, readFiles` and name your one bucket. If
it lists `deleteFiles`, or `buckets` is null, you are using a master key —
that key can delete any bucket in the account and manage other keys, so
sitting on an internet-facing host it is a larger exposure than the problem
it was meant to solve. Note that v1/v2/v3 of that endpoint reject application
keys outright, and an error body parsed carelessly will read as "no
capabilities" — a false all-clear.

The other tradeoff: `restic prune` needs delete rights, so pruning becomes a
deliberate act from a trusted machine with a separate full-capability key. At
this data size you will not need to prune for years.

**Store `RESTIC_PASSWORD` in a password manager, off the server.** It is the
encryption key for the entire repository and is kept nowhere else. Lose it
and your backups are unrecoverable noise — this is the most common way people
discover their off-site backup was never real.

### Restoring

Rehearsed end-to-end on 2026-07-19 against a real production archive; the
commands below are the ones that were actually run.

**Extract from inside a container, not from the host.** The archive stores
files owned by uid 65532 (the app's non-root user). Extracting as root on a
host that has no such user leaves files the container cannot read, and the
app fails to start in a way that looks like database corruption:

```bash
# 1. Fresh volume (use a scratch name first — rehearse before you need it)
docker volume create patchwork_data

# 2. Restore INSIDE a container so uid 65532 is preserved
docker run --rm -v patchwork_data:/data -v "$PWD/backups":/backup:ro alpine \
  tar xzf /backup/patchwork-YYYY-MM-DD.tgz -C /data

# 3. Verify ownership is 65532, not 0
docker run --rm -v patchwork_data:/data alpine ls -lan /data
```

Then verify the data before you point traffic at it:

```bash
docker run --rm -v patchwork_data:/data alpine sh -c \
  'apk add --no-cache sqlite >/dev/null
   sqlite3 /data/patchwork.db "PRAGMA integrity_check;"
   sqlite3 /data/patchwork.db "PRAGMA foreign_key_check;"
   sqlite3 /data/patchwork.db "SELECT COUNT(*) FROM nodes;"
   ls -d /data/governance/*.git | wc -l'
```

`integrity_check` should print `ok`, `foreign_key_check` should print
nothing, and the counts should match what the instance actually had —
compare against `stats` from `GET /api/v1/instance` if the old instance is
still reachable. A backup that restores but is missing rows is worse than
one that fails loudly.

Finally `docker compose up -d` and check the log for `integrity_check
passed` and `server: listening`, then confirm `/api/v1/health` returns
`{"status":"ok","db_status":"ok"}`.

If you're restoring the host config archive too, unpack it to `/opt/patchwork`
before starting the stack — the compose file reads `patchwork.yaml` and
`.env` at startup.

**Rehearse this on a throwaway target before you need it.** An archive that
has never been restored is a hypothesis, not a backup — and file-size checks
won't save you, since a subtly-empty archive is only a few KB smaller than a
good one.

**Structured export (seamrip):** admins can download a zip of all instance
data at `GET /api/v1/admin/export`, or run the export CLI from a source
checkout (`make export`). The matching `make import IN=./export/` brings the
data up on a fresh instance with new IDs. This is the data-portability path,
disaster recovery included.

## Monitoring

Nothing inside the box can tell you the box is down. A Patchwork instance is
a promise that the community is reachable, so the failure that actually hurts
is silent: the OOM killer takes the app at 03:00 and nobody notices until
someone loads the page at lunch. Three external checks cover that, and none of
them run an agent on your server — important on a 2GB box with no swap.

The reference instance splits this across two free accounts, because no
single free tier covers all three:

- [UptimeRobot](https://uptimerobot.com) free — uptime and TLS expiry. Its
  heartbeat monitor type is **Pro-only**, so it can't do the backup switch.
  (Several third-party pricing summaries claim heartbeat is on free. It
  isn't; check the vendor's own pricing page.)
- [Healthchecks.io](https://healthchecks.io) free — the backup dead-man's
  switch. 20 checks, cron-expression schedules, email alerts.

**1. Uptime — HTTP monitor on `/api/v1/health`, every 5 minutes.**

The endpoint returns `200` when healthy and `503` when degraded (currently:
the database is unreachable), so a plain status-code check is enough — no
keyword matching needed. If you change the handler, keep that property. An
endpoint that returns `200` while reporting `"status":"degraded"` in the body
reports healthy straight through an outage, which is worse than no monitor at
all because it manufactures confidence.

**2. Backups — dead-man's switch on the cron job.**

`backup-patchwork.sh` writes to a log that nobody reads. A silent failure
gives you a frozen-but-plausible backup set, which looks fine right up until
you need it. Fix that with a dead-man's switch: create a Healthchecks.io
check whose schedule is the same cron expression as the job (ours:
`15 5 * * *`, UTC) with a 2-hour grace period, then append its ping URL to
the end of the backup script:

```sh
curl -fsS -m 10 --retry 3 "https://hc-ping.com/<uuid>" >/dev/null
```

Two things make this a real success signal rather than decoration. The script
is `set -eu`, so any earlier failure exits before the ping is reached. And the
ping goes **last** — after the archive sanity check *and* after the retention
prune — so a full disk or a failing prune also withholds it. Keep both
properties if you edit the script; a ping that can fire on a failed run is
worse than none, because it certifies a backup you don't have.

**3. TLS expiry — enable SSL notifications on the HTTP monitor.**

Caddy renews Let's Encrypt certificates automatically from the `caddy_data`
volume, which holds the ACME account key. When that works you never think
about it; when it silently fails, the first symptom is a browser security
warning on day 90, and by then you have an outage and a panic. UptimeRobot
warns at 30/14/7 days out. Check the live certificate any time with:

```bash
echo | openssl s_client -servername example.org -connect example.org:443 2>/dev/null \
  | openssl x509 -noout -dates
```

**Where alerts go.** Point them at an address a human reads on a phone. Email
alone is easy to sleep through; the UptimeRobot mobile app's push
notifications cost nothing and are the difference between minutes and hours of
downtime. Don't route alerts to a shared inbox nobody owns — an alert everyone
can see is an alert nobody acts on.

## Federation (ActivityPub)

Off by default. Before enabling `federation.enabled: true`:

- `instance.domain` **must** be your real public domain. ActivityPub IDs are
  minted from it and are permanent. Federating with a wrong domain pollutes
  remote servers with unreachable IDs. The server warns at startup if
  federation is on and the domain looks local.
- Once enabled, your users and patches get actor documents, WebFinger
  resolution, and inboxes, so remote fediverse users can follow patches.
  Inbound requests are HTTP-signature verified. Outbound activities are
  signed.

Mastodon interop has been exercised in tests but not yet verified against a
live instance. Treat cross-instance federation as beta.

## Updating

Take a backup first. Database migrations run automatically at startup.

**Source-build deployment:**

```bash
git pull
docker compose up -d --build
```

**Prebuilt-image deployment** (you have a `docker-compose.override.yml`):

```bash
git pull                 # picks up compose/Caddyfile changes
docker compose pull      # fetches the new image
docker compose up -d
```

Do not add `--build` to the second one. Check which kind you have before
updating — `grep image: docker-compose.override.yml 2>/dev/null` tells you.
`docker compose config | grep -A1 patchwork:` shows what compose will
actually use after merging.

To roll back a prebuilt-image deployment, pin the previous tag in the
override file (instead of `:latest`) and re-run `docker compose up -d`.

**Updating a deployment from before 2026-07-19?** Older images had their
working directory on the ephemeral container layer, so a relative
`database.path` silently wrote *outside* the `data` volume — and recreating
the container destroyed the database. Before updating: check where your data
actually is (`docker compose exec` won't work on distroless; instead check
the volume: `docker run --rm -v patchwork_data:/data alpine ls -la /data`).
If the volume is empty but your instance has data, the database is in the
container layer — copy it out with `docker cp` before recreating, then place
it in the volume. Also `chown` pre-existing volumes to the app's uid
(`docker run --rm -v patchwork_data:/data alpine chown -R 65532:65532 /data`);
fresh volumes get correct ownership automatically from the image.

The repo ships a smoke test for exactly this failure mode —
`scripts/smoke-recreate.sh` builds the image, creates data through the API,
force-recreates the container, and verifies the data survived. Run it on any
machine with Docker if you want proof before trusting an image with real
data.

## Container limits

`docker-compose.yaml` caps log size (10MB x 3 files per container — the
Docker default is unbounded and will eventually fill the disk) and sets
`mem_limit` / `pids_limit` so a runaway process takes down its container
rather than the host. The defaults (1GB for the app, 256MB for Caddy) leave
room on a 2GB machine; raise them if you run a large instance and see the
app OOM-killed (`docker inspect --format '{{.State.OOMKilled}}'`).

The image carries a `HEALTHCHECK`. Distroless has no shell or curl, so it
runs the app's own binary in probe mode — `patchwork -healthcheck` reads the
same config the server does and checks `GET /api/v1/health`, which returns
503 when the database is unreachable. `docker compose ps` shows the state,
and `docker inspect --format '{{json .State.Health}}' <container>` shows the
last few probe results with their output.

Docker only *reports* health; it doesn't restart an unhealthy container. It
complements the external uptime check in [Monitoring](#monitoring) rather
than replacing it — an unreachable host is the failure a probe inside that
host can never report.

## Hardening the host

Patchwork's container story is reasonable out of the box — distroless,
non-root, only Caddy publishes ports. The host underneath it is your job.
This section covers the four things that actually matter on a small
single-box deployment, and one popular measure that mostly doesn't.

### SSH: get to key-only, safely

Password authentication on port 22 is the surface worth closing first. Public
IPs get scanned within minutes of going live, and the guesses are often
*targeted* — expect your instance name as an attempted username, not just
`admin`.

**Audit before you disable, or you can lock yourself out.** Find out whether
any account actually relies on a password:

```bash
# Accounts with a real password hash (not ! or * = disabled)
sudo awk -F: '($2 !~ /^[!*]/) && ($2 != "") {print $1}' /etc/shadow
# Accounts that can log in at all
awk -F: '($3>=1000 || $3==0) && $7 !~ /(nologin|false)/ {print $1, $7}' /etc/passwd
```

If the first command prints nothing, no one can password-log-in anyway and
disabling it costs you nothing. If it prints a name, make sure that person
has a key installed *before* you continue.

Use a drop-in rather than editing `sshd_config`, so the change is one file to
delete if you need to revert:

```bash
sudo tee /etc/ssh/sshd_config.d/10-hardening.conf >/dev/null <<'CONF'
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
PermitRootLogin prohibit-password
CONF
sudo sshd -t   # syntax check — never skip this
```

**Keep a second session open.** `sshd -t` catches syntax errors but not
"my key isn't where I thought it was." Leave your current SSH session
connected, `sudo systemctl reload ssh` (reload, not restart — it won't drop
existing connections), then open a *separate* terminal and confirm you can
still get in. Only close the first session once the second one works.

Verify both directions:

```bash
ssh you@host 'echo ok'                                    # should succeed
ssh -o PreferredAuthentications=password -o PubkeyAuthentication=no you@host
# should fail with: Permission denied (publickey).
```

### A firewall that actually covers Docker

Here is the trap: **Docker publishes ports by writing rules into the `DOCKER`
iptables chain, which is evaluated before — and independently of — ufw's
`INPUT` chain.** So this sequence:

```bash
sudo ufw allow 22,80,443/tcp && sudo ufw enable   # looks right, isn't
```

produces a firewall that reports `Status: active` with a tidy rule list while
every Docker-published port stays reachable from the internet regardless of
what ufw says. That is worse than no firewall, because it manufactures
confidence you haven't earned. If you take one thing from this section, take
this one.

Two honest options:

- **Use your provider's network firewall** (Hetzner Cloud Firewall, AWS
  security groups, DigitalOcean Cloud Firewalls). It filters upstream of the
  host, so Docker's iptables rules cannot bypass it, it survives host
  misconfiguration, and it costs no RAM. On a small box this is the better
  tool. Allow 22, 80, 443 inbound; default-deny the rest.
- **If you need it on-host**, write rules into the `DOCKER-USER` chain (which
  *is* consulted for container traffic), or use `ufw-docker`. Plain ufw rules
  alone will not do it.

Either way, verify from somewhere else rather than trusting the status
output — `nmap` from another machine, or an external port scanner. Check that
Patchwork's app port is not among the results: in the shipped compose file
the app uses `expose:` rather than `ports:`, so only Caddy is reachable, and
a scan should show 22, 80, 443 and nothing else.

### Confirm automatic updates are actually applying

`unattended-upgrades` being installed and `active` does not mean it has ever
done anything. Two independent failure modes, and "0 pending updates" looks
identical to healthy in both:

```bash
systemctl list-timers 'apt-daily*' --all   # LAST column "-" = never fired
sudo tail /var/log/unattended-upgrades/unattended-upgrades.log
```

A fresh cloud image is the common case: the timers haven't fired yet, and the
image itself may ship a kernel a year or more out of date. Nothing is broken;
it just hasn't run. Prime it once by hand:

```bash
sudo apt update && sudo apt full-upgrade
```

**Kernel and libc updates need a reboot to take effect.** A patched kernel
sitting on disk while the old one is still running is not a patched system —
and unattended-upgrades will not reboot for you unless you set
`Unattended-Upgrade::Automatic-Reboot`. Check with:

```bash
[ -f /var/run/reboot-required ] && cat /var/run/reboot-required.pkgs
```

If a major `docker-ce` version bump is in the pending set, consider
`sudo apt-mark hold docker-ce docker-ce-cli containerd.io` for that round and
doing Docker as its own separate change. Bundling a daemon major-version jump
with a kernel reboot means that if the site doesn't come back, you won't know
which one to blame.

### Test the cold boot before you need it

The `restart: unless-stopped` policy in the compose file is supposed to bring
Patchwork back after a reboot without anyone logging in. Most operators never
verify this, and then discover it during an unplanned power event at 3am.

Reboot deliberately, once, while you're watching — then confirm recovery
*without* running `docker compose up`:

```bash
uname -r                                       # new kernel is running
docker ps                                      # both containers, no manual start
systemctl --failed                             # empty
curl -sS -o /dev/null -w '%{http_code}\n' https://your.domain/
```

Also confirm your TLS certificate came back rather than being re-issued —
compare the serial before and after. Caddy stores certs in the `caddy_data`
volume; if that volume were missing, Caddy would silently request a fresh
cert on every boot and eventually hit Let's Encrypt rate limits.

```bash
echo | openssl s_client -connect your.domain:443 -servername your.domain 2>/dev/null \
  | openssl x509 -noout -serial -dates
```

Same serial before and after = the volume persisted correctly.

### On fail2ban: probably skip it

Conventional hardening advice says add fail2ban. Think about what it buys you
*after* the steps above. With password authentication off, every brute-force
attempt fails at the protocol level no matter how many times it's tried —
fail2ban would be banning attackers who had no path in to begin with. What
you get is quieter logs, and what you pay is a Python daemon (~20MB resident)
plus another moving part, on a box that may have 2GB of RAM and no swap.

Worth adding if you have other password-authenticated services, or if the log
volume is genuinely costing you disk. Otherwise the honest answer for a small
community instance is that key-only SSH already did the job, and this is
ritual rather than defense. Skipping something that doesn't help is a
legitimate hardening decision — spend the complexity budget on backups you've
actually tested restoring instead.

## Troubleshooting

**`docker compose up` fails with "config file not found."** You didn't create
`patchwork.yaml`. Copy the example: `cp patchwork.yaml.example
patchwork.yaml`. (If a *directory* named `patchwork.yaml` showed up instead,
Docker created it when the file was missing. Delete it and create the real
file.)

**Startup fails with "database path ... is NOT on a mounted volume."** The
durability guard: in a container, a database outside a volume is destroyed
by the next recreate, so the app refuses to run that way. Keep the compose
file's `data:/data` volume mount, and either use a relative `database.path`
(resolves under `/data`) or an absolute path inside `/data`. For an
intentionally throwaway instance, set `PATCHWORK_ALLOW_EPHEMERAL=1`.

**Startup fails with "create data dir: permission denied."** The volume
predates the ownership fix and is owned by root. Fix:
`docker run --rm -v patchwork_data:/data alpine chown -R 65532:65532 /data`.

**No HTTPS, or certificate errors.** Check that `DOMAIN` is set in `.env`,
DNS points at this server, and ports 80+443 are open. `docker compose logs
caddy` shows the ACME conversation.

**Magic link never arrives.** Without SMTP it's sitting in the app log:
`docker compose logs patchwork | grep auth/verify`. With SMTP, check the log
for `send magic link email` errors. Both submission ports work: 465 uses
implicit TLS, 587 uses STARTTLS — if your provider documents both, either is
fine, but make sure `smtp.port` matches the one you actually opened in your
firewall (see [Email](#email-optional)).

**I locked myself out, or no admin exists.** If the instance has real users
but no admin (the first-account rule only applies to an empty instance),
promote one directly:

```bash
docker run --rm -it -v patchwork_data:/data alpine sh -c \
  "apk add -q sqlite && sqlite3 \$(find /data -name patchwork.db) \
   \"UPDATE users SET role='admin' WHERE username='YOURNAME';\""
docker compose restart patchwork
```

(The `find` handles both layouts: the default relative config puts the file
at `data/patchwork.db` inside the volume; an absolute
`database.path: /data/patchwork.db` puts it at the volume root.)

**Wrong instance name showing.** You're running with an unedited config. The
startup log prints warnings when config values still look like the example
file.
