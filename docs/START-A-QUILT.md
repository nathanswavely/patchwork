# Start a Quilt

This page is for community organizers deciding whether Patchwork is for
them. It assumes no technical background. If you already know you want to
deploy one and how, you want [DEPLOYMENT.md](DEPLOYMENT.md) instead.

## What Patchwork is

Patchwork is a platform for the communities of one place — a city, a
scene, a region — to organize, publish events, govern themselves, and
find each other. Every group is a **patch**: a band, a venue, a book
club, a mutual aid network, all equal. The patches together form a
**quilt** — a live picture of your community where groups that share
people sit closer together, because shared people are what connection
actually is.

Nobody is selling ads. No algorithm decides what anyone sees. The
software is open source, and each quilt is run by a person or group in
its own community, on a machine that community controls.

What it is not: a social network you post on all day, a Facebook
replacement for individuals, or a service someone else hosts for you.
There is no company behind your quilt — that is the point, and it means
someone in your community has to hold it.

## What running a quilt asks of you

Be honest with yourself about this part before anything else.

**A steward.** Every quilt names at least one person publicly
accountable for how it is run — that's the Label, a page on every quilt
stating who stewards it, what it depends on, and what it costs. If
nobody in your community is willing to be named, you are not ready yet,
and that is a fine thing to learn now.

**A baseline you inherit.** Every patch on every quilt starts from the
lining — a shared community-standards baseline that ships with
Patchwork and includes an anti-discrimination floor. It is the
project's statement of the bare minimum a quilt should be. Patches can
amend their copy, but amendments are public and visibly marked. If your
organization's values conflict with the lining, Patchwork is not your
tool.

**Judgment, occasionally.** Stewards handle reports, review the odd
event submission, and — rarely — deal with a patch or person acting in
bad faith. The software gives you the levers; pulling them is human
work. Expect a few hours a month once things are calm, more during
launch.

## What it costs

- **A machine.** Patchwork is a single small program. It runs on a
  five-dollars-a-month virtual server, or a Raspberry Pi on a shelf.
- **A domain name.** Ten to twenty dollars a year.
- **Email sending (optional).** Sign-in links can go over email, but
  Patchwork works without it — admins can hand out invite links over
  Signal, or on paper. Add email when you want it.

That's the whole bill. The Label on your quilt states your actual
numbers to your community, so the people who benefit can help carry it.

## Ways to get one running

1. **Someone technical in your community.** The honest first answer.
   Deployment is a well-worn path (Docker, one config file, automatic
   HTTPS) — an afternoon for someone who has run a web service before.
   [DEPLOYMENT.md](DEPLOYMENT.md) is the map.
2. **You, carefully.** If you can rent a small server and follow a
   guide patiently, the path is documented end to end. The software is
   deliberately boring to operate: one process, one database file,
   backups are copying a file.
3. **Ask.** Open an issue on the project repository. The people who
   build Patchwork would rather help a real community launch than add a
   feature.

## The promises that keep it honest

Patchwork is built so that trust in any one person — including whoever
runs your quilt — is never load-bearing:

- **Open source.** The software belongs to nobody and everybody. No
  company can shut it down, acquire it, or start charging rent.
- **The seamrip.** Every quilt's data can be exported and stood up
  again elsewhere. If leadership goes sideways, the community can take
  its patches, its people, and its history, and leave. The threat of
  leaving is what keeps stewards honest — including you.
- **The Label.** Costs and dependencies are disclosed on every quilt,
  so "who pays for this and what do they control" is never a mystery.

## Where to go next

- Browse a living quilt: the reference instance is the Lancaster, PA
  arts scene.
- Read [DEPLOYMENT.md](DEPLOYMENT.md) when you're ready to try it on a
  server.
- The [README](../README.md) covers the technical shape of the project.
