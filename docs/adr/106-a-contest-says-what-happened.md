# 106. A contest says what happened

**Status:** accepted, 2026-09-15

## Context

Epoch 4 of the governance simulation (docs/adr/096) ran the first council
election anybody actually stood in: a three-seat contest on a co-op that had
spent a year with an empty council, eight members, two candidates, four
ballots cast, one person seated. The mechanics held. What did not hold was
what the product *said* about them, and five of the eight people said so
independently.

**"The council continues until a successor is elected" was printed to a
patch with no council.** It is docs/adr/051's holdover rule and it is the
right sentence for a council that exists. Bobbin Hall had none: the
inactivity sweep had emptied every chair (the churn docs/adr/102 fixed), five
contests in a row settled nothing, and each one told the members the council
was carrying on. The governance page two inches away said "Nobody holds the
admin role here — 0 held and 3 vacant". Priya: *"Two of your sentences told me
for nine months that the council continues, and there has been no council at
all. I would have come back in January."* Teo named the cost precisely:
*"Reading a comforting sentence I know to be false, five times in a row, is
what made me stop trusting the rest of the page — including the bits that were
telling me the truth."*

**A ballot opened over a slate with nobody on it, and the whole electorate was
called to it.** Nothing looked at the candidates when nominations closed. Two
simulated contests sent twelve people *"Voting is open. Approve as many
candidates as you like"* over empty slates, then ran a fortnight to reach the
answer that was already true. Sam, who had failed twice to find a way onto a
board, got one of them: *"If I had clicked that one at the time, I'd have
arrived at an empty list and concluded, again, that this thing doesn't work
and there's no way in."*

**When nominations closed, the page erased that they had happened.** The
closing date was rendered only inside the `nominating` branch, and the Stand
button lived in the same guard, so the window and the control vanished
together. Three members arrived to stand and could not learn from the product
that a window had existed, when it shut, or what to do now. Teo: *"the button
gone without a trace"*. It cost a real candidacy — Devon had decided to stand.

**Saving a ballot said nothing at all.** All four voters went looking for
proof somewhere else and three said they would have voted again to be safe.
Rosa: *"the only proof I voted was a number quietly going up by one, and then
it went up by one again and I spent ten minutes wondering if that second one
was me."*

**And the member seamrip's blurb described a smaller file than the button
hands over.** It promised nothing "from a patch you are not in"; the archive
holds every public patch on the quilt, which is what `memberview.go`
implements and what the README inside the zip says. Nell, who took one:
*"the sentence I was given before I pressed the button and the file I got
afterwards do not describe the same thing."*

## Decision

**A sentence about the council asks the council first.** `holdoverLine`
reads the seats at the moment a contest settles nothing. With somebody still
in a chair it says the true, reassuring thing — the council continues. With
every chair empty it says so: nobody was elected, the seats are still empty,
this patch has no admins. docs/adr/102 made an empty council a state the
product accepts; this is the product admitting to it at the moment it
happens, in the notification, the first time rather than the fifth.

**The two surfaces that are read later stop claiming anything about the
council at all.** The governance Record and the proposal banner outlive the
council they would be describing and cannot know what it was on the day, so
they say what the *contest* did — "Settled nothing. Nobody was elected." —
and let the council block, which is live, say what the council is. A record
that narrates a meeting carrying on in an empty room is worse than one that
narrates nothing.

**A contest nobody stood in settles when nominations close.** The outcome is
identical to letting the ballot run — holdover, nobody seated, the chairs
released (docs/adr/103) — and it is reached without inviting anybody to a page
that cannot have a result. A fortnight and a notification each are not the
price of a conclusion already reached.

**A window is its closing date, and a closed door says so.** The banner names
the day standing shuts while nominations are open; once they have closed, the
panel says "Standing closed <date>" in every later phase. Somebody who arrives
late learns that they are late, and when.

**Saving a ballot says so**, and whether a ballot is in is read from the
server (`approved_by_me`) rather than only from the click, so it survives the
reload that three of four voters reached for. The button becomes "Update my
ballot", and the confirmation replaces the standing advice rather than sitting
above it repeating the offer to change your mind.

**The seamrip's blurb describes the file.** Every public patch on the quilt
and the private ones you belong to, with their events, charters, member lists
and proposals; no email addresses, contact cards or noticeboards. One
sentence, not a boundary change — the boundary was right.

## Consequences

- Two regression suites now pin sentences rather than mechanics, which is
  unusual here and deliberate: every defect in this decision was a true
  system telling a person something false, and nothing in the old suites
  could go red for that.
- `TestSweepElections_SkipsArchivedPatches` and
  `TestGovernanceOverview_SurfacesTheLiveElection` each opened a ballot over
  an empty slate incidentally. Both now stand a candidate first. Neither test
  was about the slate.
- An election with no candidates now closes roughly a fortnight earlier than
  it used to, so the breather before the calendar retries (docs/adr/051) also
  starts a fortnight earlier. That shortens the cycle on a patch nobody is
  standing in — which is the patch least in need of a long wait, but it is a
  change in cadence and is worth watching against F-098, where the cadence is
  already the complaint.
- Nothing here gives anybody a way to nominate somebody else, or to withdraw
  a candidacy, and four surfaces still promise the first (F-088). A member
  put herself on a ballot by accident this epoch following that promise and
  could not get off. That is the next piece of work and it is a route, not a
  sentence.
