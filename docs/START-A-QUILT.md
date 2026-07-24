# Start a Quilt

This page is for community organizers deciding whether Patchwork fits
them. It assumes no technical background. If you already know you want
to deploy one and how, go straight to [DEPLOYMENT.md](DEPLOYMENT.md).

## What Patchwork is

Patchwork is a platform for the communities of one place. A city, a
scene, a region. Every group on it is a **patch**: a band, a venue, a
book club, a mutual aid network, all equal. Together the patches form a
**quilt**, a live picture of your community where groups that share
people sit closer together, because shared people are what connection
actually is.

Nobody sells ads here. No algorithm decides what anyone sees. The
software is open source, and each quilt runs on a machine its own
community controls, administered by a person that community can name.

It helps to know up front what Patchwork is a poor fit for. It is not a
social network you post on all day, and there is no company behind it,
so nobody hosts it for you. Someone in your community has to hold it,
and that requirement is the design working as intended, since a
community that runs its own infrastructure answers to nobody upstream.

## What running a quilt asks of you

Be honest with yourself about this part first.

**A steward.** Every quilt names at least one person publicly
accountable for how it runs. That's the Label, a page on every quilt
stating who stewards it, what it depends on, and what it costs. If
nobody in your community will be named, you are not ready yet, and it
costs nothing to learn that now.

**A baseline you inherit.** Every patch on every quilt starts from the
lining, a shared community-standards baseline that ships with Patchwork
and includes an anti-discrimination floor. The lining is the project's
statement of the bare minimum a quilt should be. Patches can amend
their copy, but amendments are public and visibly marked. If your
organization's values conflict with the lining, Patchwork is the wrong
tool for you.

**Judgment, occasionally.** Stewards handle reports, review the odd
event submission, and once in a while deal with a patch or person
acting in bad faith. The software gives you the levers. Pulling them is
human work. Expect a few hours a month once things settle, more during
launch.

## What it costs

A machine, a domain name, and optionally email sending. Patchwork is
one small program that runs happily on a five-dollar-a-month virtual
server or a Raspberry Pi on a shelf, and a domain runs ten to twenty
dollars a year. Sign-in links can go out over email, but Patchwork
works without it: admins can hand out invite links over Signal or on
paper, and you can add email whenever you want it.

That is the whole bill. Your quilt's Label states your actual numbers
to your community, so the people who benefit can help carry them.

## Ways to get one running

The honest first answer is someone technical in your community.
Deployment is a well-worn path with Docker, one config file, and
automatic HTTPS, roughly an afternoon for anyone who has run a web
service before. [DEPLOYMENT.md](DEPLOYMENT.md) is the map.

The careful second answer is you. If you can rent a small server and
follow a guide patiently, the path is documented end to end, and the
software is deliberately boring to operate. One process. One database
file. Backups are copying a file.

If neither fits, open an issue on the project repository and ask. The
people who build Patchwork would rather help a real community launch
than ship another feature.

## The promises that keep it honest

Trust in any one person, including whoever runs your quilt, is never
load-bearing here. The software is open source, so no company can shut
it down or start charging rent. Every quilt's data can be exported and
stood up again elsewhere, an act the vocabulary here calls a seamrip:
if leadership goes sideways, the community takes its patches, its
people, and its history somewhere new. Costs and dependencies sit on
the public Label, so "who pays for this and what do they control" has a
published answer. Stewards stay honest because leaving is real, and
once you hold the keys that pressure applies to you too.

## Where to go next

Browse a living quilt first if you can. The reference instance serves
the arts scene of Lancaster, Pennsylvania. When you're ready to try the
software on a server, read [DEPLOYMENT.md](DEPLOYMENT.md). The
[README](../README.md) covers the technical shape of the project.
