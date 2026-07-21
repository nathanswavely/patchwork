package handler

// Default legal documents (docs/adr/028). Every deployment serves these
// from first boot; the instance admin can replace either wholesale from
// the admin panel (stored in instance_settings, docs/adr/014 pattern).
//
// These templates describe what the SOFTWARE actually does — every claim
// here is checked against the codebase. If a feature changes in a way
// that falsifies a sentence below (e.g. adding analytics, changing what
// federates, adding self-serve account deletion), update the sentence in
// the same PR. A privacy policy that drifts from the code is worse than
// none.
//
// {quilt_name} and {domain} are substituted at serve time with the
// effective instance name (DB override or patchwork.yaml) and configured
// domain, so a rename never strands a stale name inside a legal document.
//
// Formatting: one line per paragraph and per list item — the frontend
// renders markdown with breaks:true (single newlines become <br>), so a
// wrapped paragraph would render ragged.

const defaultPrivacyPolicy = `*This is the default privacy policy that ships with the Patchwork software. It describes what the software actually does. The people who run {quilt_name} can replace it with their own, and if they have, this notice won't be here.*

## The short version

Nobody runs {quilt_name} for profit. There are no ads or trackers here, no analytics scripts, and nothing about you gets sold or rented to anyone. The site keeps the minimum it needs to work: an email address to sign you in, the profile you choose to write, and a record of which patches you join and which events you RSVP to. Real people run this server. The [Label](/label) says who they are and how to reach them.

## What this site collects

**Account information.** Your email address, which is used to send sign-in links and any notifications you turn on, plus your chosen username. A display name, bio, and avatar are stored only if you add them. Passkeys are stored as public keys. The private key never leaves your device, and there are no passwords anywhere in the system.

**Activity.** The patches you join, follow, or administer, the events you RSVP to, the proposals and votes you take part in, and anything you post or edit.

**Technical records.** Signing in stamps your session with an IP address. Administrative actions go into an audit log along with the acting account and IP address, and the web server keeps ordinary request logs. All of this exists for security and troubleshooting. None of it is used for profiling.

## What other people can see

Profiles are public pages showing your username, display name, bio, avatar, and whichever memberships you have chosen to show. Each membership has its own visibility switch, and you hold it. The same switch controls both your profile and the patch's public member list, so the two never disagree. A membership you hide stays visible to that patch's admins and members inside the patch workspace, since the people you organize with can already see you in the room. The list of patches you follow never appears on your profile.

Your email address is never shown publicly. Only this site's administrators can see it.

## Federation: where your public content travels

This site can speak ActivityPub, the protocol behind Mastodon and similar networks. With federation enabled, public content can be copied to other servers when someone there follows a patch here. That covers patch profiles, public events, and public posts. Two things to understand:

- Other servers keep their own copies under their own policies. Deleting something here sends a deletion notice, but this site cannot force another server to honor it.
- When you follow a patch on another quilt, the follow is relayed through this site's shared service account rather than your own identity, so your name never appears in a remote follower list.

Membership records never federate. What leaves this server is only what was already public on it.

## Cookies

This site sets one cookie. It's the HTTP-only session cookie that keeps you signed in. That's it. There are no advertising or third-party cookies here, and no cross-site tracking of any kind.

## Email

Sign-in links and any notifications you turn on go out through the email server this site's stewards configured. Email always passes through the providers involved on both ends, yours and this site's.

## The seamrip: data portability

Patchwork is built so a community can pack up and leave. Site administrators can export the instance's data, membership records included, to start a successor site. The software documents this on purpose as a governance safety valve. If this community's leadership goes sideways, the community can fork itself under new stewards, and the connections between people and patches survive the move. Sessions, passkeys, email delivery settings, and federation keys never travel in an export.

## How long things are kept, and deletion

Your content and account stay until removed. There is no self-serve deletion button yet. To have your account or specific content removed, contact the stewards listed on the [Label](/label), and they can remove it with the administrative tools. Content that federation already copied to other servers, or that went out in an export made before the removal, may persist outside this server's control.

## Age

This site is not directed at children under 13, and nobody under 13 may create an account.

## Changes

If this policy changes, the change shows up on this page. The software keeps no hidden version of it. What you read here is what the site operates under.

## Who to talk to

The stewards named on the [Label](/label) run {quilt_name}. For any question about your data, they are the people to ask.`

const defaultUserAgreement = `*This is the default user agreement that ships with the Patchwork software. The people who run {quilt_name} can replace it with their own, and if they have, this notice won't be here.*

## The short version

Be someone your community would vouch for. What you post stays yours, the stewards can moderate the site, and nobody here promises uptime. If you ever stop trusting how the place is run, the software guarantees your community the right to leave with its data.

## What this is

{quilt_name} is a community site run by the people named on the [Label](/label), called the stewards from here on, using the open-source Patchwork software. Nobody operates it as a company or a commercial service. Creating an account or using the site means you agree to what's on this page.

## Who can join

You must be at least 13 years old to have an account. Accounts are for people. Bots and automated posters don't get one.

## Your account

Sign-in is by email links and passkeys, so there is no password to guard. The email account and devices you use to sign in are still your responsibility, and what happens through your account is yours to answer for. Don't share access to your account or use someone else's.

## Your content

What you post is yours, and you keep every right you already had. So the site can function, you give {quilt_name} permission to store your content, show it to the people it's addressed to, and, where federation applies to public content, pass it along to other servers that follow this one. The same permission covers the seamrip: if this community exports its data to continue elsewhere, public content and membership records go with it, as described in the privacy policy. Anything you post publicly can be seen, copied, and shared the way public things on the internet are, and you agree to that too.

## Conduct

These rules apply everywhere on this site:

- No harassment, and no content that attacks people for who they are. Racism, antisemitism, misogyny, homophobia, transphobia, ableism, and fascist organizing are not welcome here. That commitment is written into the software itself, and this site keeps it.
- No illegal content, no spam, no impersonation, and no attempts to break or abuse the platform or other people's accounts.
- Individual patches can set extra rules for their own spaces through their linings and governance. Joining a patch means playing by its rules.

## Moderation

The stewards can remove content, restrict accounts, or close accounts that violate this agreement, without prior notice where they judge that necessary to protect people or the site. Posting content that infringes someone else's copyright will, if repeated, cost you your account. To report copyright infringement or any other abuse, contact the stewards through the [Label](/label).

## If you don't like how this place is run

Disagreement is not a violation. Patchwork keeps governance visible and makes leaving real: any member can export what they can already see, and a community can seamrip itself to new stewards. The [Label](/label) explains how this site is run, what it costs, and where the exit is.

## No warranty

Volunteers run this site on real hardware, with no promise of uptime, availability, or permanence. It is provided **as is**, without warranty of any kind. To the fullest extent permitted by law, the stewards are not liable for damages arising from your use of the site or from content other people post, so back up anything you can't afford to lose.

## Changes and ending

This agreement can change, and the current version is always this page. Using the site after a change means accepting it. You can stop using the site whenever you like, and the stewards can end the agreement on their side by closing your account as described under Moderation.`
