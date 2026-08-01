/**
 * Amendments adopted elsewhere (docs/adr/053).
 *
 * The traps these guard:
 *
 *   - A patch that decides at meetings must not be shown vote buttons. The
 *     server refuses the ballot; a page that still offers it is the
 *     offer-then-403 dead end docs/adr/044 was written to end.
 *   - Recording is offered only where the venue is declared. Without that
 *     gate, "the community adopted this" becomes a button beside a vote it
 *     could be used to go around.
 *   - The lining is never attestable (docs/adr/037), and the check has to key
 *     on `kind`, not on the title — the column exists precisely so the title
 *     isn't load-bearing.
 *   - An attestation replaces the whole text. A form that reads like a patch
 *     would produce a charter that is half of two documents.
 *
 * There is no Svelte render library in this project (see router.test.js,
 * patch-setup.test.js), so component wiring is asserted against source text.
 */
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function source(relPath) {
  return readFileSync(resolve(process.cwd(), 'src', relPath), 'utf8');
}

describe('AdoptedElsewhere — the record', () => {
  const src = source('components/AdoptedElsewhere.svelte');

  it('reads the records without gating on membership', () => {
    expect(src).toMatch(/api\(`nodes\/\$\{slug\}\/amendment-attestations/);
    expect(src).not.toMatch(/isMember/);
  });

  it('takes the document filename from the payload rather than reslugifying it', () => {
    // governanceFilename is the server's rule (docs/adr/011). A second copy of
    // it here would drift, and a per-document view would silently ask for the
    // wrong document — showing an empty history rather than an error.
    expect(src).toMatch(/docFilename = ''/);
    expect(src).not.toMatch(/toLowerCase\(\)\.replace/);
  });

  it('gates recording on the declared venue, read from the rules', () => {
    expect(src).toMatch(/decidesElsewhere = \(rules\?\.proposal_venue \|\| 'patchwork'\) === 'elsewhere'/);
    expect(src).toMatch(/\{#if isAdmin && decidesElsewhere\}/);
  });

  it('records through step-up, because it rewrites the charter', () => {
    expect(src).toMatch(/withStepUp\(\(\) =>/);
    expect(src).toMatch(/<PasskeyNotice show=\{!hasPasskey\}/);
  });

  it('asks for the whole text and says so', () => {
    expect(src).toMatch(/This replaces the whole document/);
    expect(src).toMatch(/not just\s*\n?\s*the part that changed/);
  });

  it('renders nothing on a patch that has neither the venue nor a record', () => {
    expect(src).toMatch(/\{#if !loading && \(decidesElsewhere \|\| records\.length > 0\)\}/);
  });
});

describe('GovernanceDetail — where an adopted text is recorded', () => {
  const src = source('pages/GovernanceDetail.svelte');

  it('never offers it on the lining, and decides that by kind not by title', () => {
    // docs/adr/037: the only thing that changes a lining's body is a passed
    // amendment proposal. The lining is identified by column, so a title check
    // here would be the exact mistake that column exists to prevent.
    expect(src).toMatch(/\{#if doc\.kind !== 'lining'\}/);
    expect(src).not.toMatch(/doc\.title === 'Community Standards'/);
  });

  it('passes the filename the server gave it', () => {
    expect(src).toMatch(/docFilename=\{doc\.filename\}/);
  });
});

describe('GovernanceList — recording a document Patchwork does not have', () => {
  const src = source('pages/GovernanceList.svelte');

  it('mounts the panel with no document, so a new one can be named', () => {
    expect(src).toContain('<AdoptedElsewhere {slug} {isAdmin}');
  });
});

describe('ProposalStatusBanner — a proposal with no ballot', () => {
  const src = source('components/ProposalStatusBanner.svelte');

  it('says the patch decides elsewhere rather than showing a voting banner', () => {
    expect(src).toMatch(/\{:else if effectiveState === 'elsewhere'\}/);
    expect(src).toMatch(/decides at meetings, not here/);
  });

  it('still lets the author withdraw, because the proposal is open', () => {
    const branch = src.slice(src.indexOf("effectiveState === 'elsewhere'"), src.indexOf("effectiveState === 'approved'"));
    expect(branch).toMatch(/Withdraw this proposal/);
  });
});

describe('ProposalDetail — the diff whose ground moved', () => {
  const src = source('pages/ProposalDetail.svelte');

  it('tells a draft when the document was adopted underneath it', () => {
    // docs/adr/053 takes the trade that an attestation checks no base. The
    // notice is what that trade owes the draft's readers.
    expect(src).toMatch(/\{#if proposal\.ground_moved\}/);
    expect(src).toMatch(/compare against the older text/);
  });

  it('keeps the vote surfaces off, since `elsewhere` is not a voting state', () => {
    expect(src).toMatch(/isVoting = \$derived\(effectiveState === 'voting'/);
  });
});

describe('StructuredRulesEditor — the second venue', () => {
  const src = source('components/StructuredRulesEditor.svelte');

  it('offers where proposals are decided, alongside where admins are chosen', () => {
    expect(src).toMatch(/proposalVenue = currentRules\.proposal_venue === 'elsewhere' \? 'elsewhere' : 'patchwork'/);
    expect(src).toMatch(/proposal_venue: proposalVenue/);
    expect(src).toMatch(/Where proposals are decided/);
  });

  it('tracks it for the auto-save effect, or the choice never reaches the parent', () => {
    expect(src).toMatch(/subjectRecusal; leadershipVenue; proposalVenue;/);
  });
});

describe('StructuredRulesDiff — nothing changes invisibly', () => {
  const src = source('components/StructuredRulesDiff.svelte');

  it('diffs every key either side carries, not only the labelled ones', () => {
    // The rules file holds fields this editor does not edit. A diff that only
    // knew its own labels would let one of them change while the page reported
    // the rest unchanged — the worse half of a governance record.
    expect(src).toMatch(/allKeys = \$derived\(\[\.\.\.new Set\(\[/);
    expect(src).toMatch(/\.\.\.Object\.keys\(currentRules \|\| \{\}\)/);
    expect(src).toMatch(/LABELS\[key\] \|\| key\.replace/);
  });
});

/**
 * An election's banner (docs/adr/051).
 *
 * The trap: an election row carries `state = 'voting'` from the moment the
 * calendar creates it, because that is where it is headed — not where it is.
 * A banner reading the state announced an open vote for the whole two-week
 * nomination window, directly above a panel correctly saying nominations
 * close on the 15th, and with no voting_ends_at it rendered the empty time
 * as "Voting is open. . Cast your vote below."
 *
 * The phase is the truth; the state is the destination.
 */
describe('ProposalStatusBanner — an election in nominations', () => {
  const src = source('components/ProposalStatusBanner.svelte');

  it('branches on the phase before the state', () => {
    expect(src).toMatch(/\{#if electionPhase === 'nominating'\}/);
    expect(src).toMatch(/Nominations are open\. Voting starts when they close\./);
    // The phase branch has to come first, or the state branch swallows it.
    expect(src.indexOf("electionPhase === 'nominating'")).toBeLessThan(
      src.indexOf("effectiveState === 'voting'")
    );
  });

  it('never renders an empty time-left as a bare full stop', () => {
    expect(src).toMatch(/timeLeft \? `Voting is open\. \$\{timeLeft\}\.` : 'Voting is open\.'/);
  });

  it('offers no withdrawal on an election, whoever the record names as author', () => {
    // `systemAuthorFor` names the longest-standing admin because the record
    // needs a name — a stand-in for the calendar, not somebody who raised a
    // proposal. Nobody calls an election and nobody closes one.
    expect(src).toMatch(/mayWithdraw = \$derived\(isAuthor && !electionPhase\)/);
    expect(src).not.toMatch(/\{#if isAuthor\}/);
  });
});

describe('ProposalDetail — what it hands the banner', () => {
  const src = source('pages/ProposalDetail.svelte');

  it('passes the election phase, or the banner cannot tell the two apart', () => {
    expect(src).toMatch(/electionPhase=\{proposal\.election_phase \|\| ''\}/);
  });
});

describe('GovernanceOverview — what "since" means', () => {
  const src = source('components/GovernanceOverview.svelte');

  it('does not claim an admin-since date nothing records', () => {
    // joined_at is when someone joined the patch, in whatever role. Under an
    // "Admin since" label it read as a governed fact and was routinely false:
    // a member of eight months elected this morning showed eight months.
    expect(src).toMatch(/Member since \{formatDate\(admin\.joined_at\)\}/);
    expect(src).not.toMatch(/Admin since \{formatDate/);
  });
});
/**
 * The governance hub can see the election (docs/adr/051).
 *
 * The trap this guards: the needs-a-vote banner stays quiet during nominations
 * on purpose — nominations are not a ballot, and counting one there sends
 * people to a page with no vote buttons. That left the hub silent for the
 * whole nomination window, which is the only stretch when standing or putting
 * someone forward is possible.
 */
describe('GovernanceOverview — a live election', () => {
  const src = source('components/GovernanceOverview.svelte');

  it('takes the contest and the term date from the server, not from dates on hand', () => {
    expect(src).toMatch(/election = \$derived\(overview\?\.election \|\| null\)/);
    expect(src).toMatch(/nextTermEnd = \$derived\(overview\?\.next_term_end \|\| ''\)/);
  });

  it('calls for nominations while they are open, and says how many are standing', () => {
    expect(src).toMatch(/\{#if election\?\.phase === 'nominating'\}/);
    expect(src).toMatch(/Nominations are open for \{election\.seats\}/);
    expect(src).toMatch(/Nobody has stood yet\./);
  });

  it('asks a member to stand and everyone else only to look', () => {
    // Standing is a member act, the same gate as raising a proposal. A
    // follower offered "stand, or put someone forward" gets a 403 on click.
    expect(src).toMatch(/canPropose \? 'Stand, or put someone forward' : 'See who is standing'/);
  });

  it('says a ballot is open once nominations close', () => {
    expect(src).toMatch(/\{#if election\?\.phase === 'voting'\}/);
    expect(src).toMatch(/A ballot is open for \{election\.seats\}/);
  });

  it('shows when the next seat comes up, and that a lapsed term removes nobody', () => {
    expect(src).toMatch(/Next seat comes up \{formatDay\(nextTermEnd\)\}/);
    expect(src).toMatch(/It serves until a successor is elected\./);
  });

  it('reads a lapsed term as a calendar date, not a UTC instant', () => {
    // Same trap as formatDay: a bare YYYY-MM-DD parsed as UTC midnight reads
    // as lapsed a day early west of Greenwich.
    expect(src).toContain('const parts = /^(\\d{4})-(\\d{2})-(\\d{2})$/.exec(nextTermEnd)');
  });
});

/**
 * What a patch has decided, in order (docs/adr/055).
 *
 * The record assembles from proposals and attestations rather than storing
 * anything, so the tests worth having are about what it claims. The sharp one:
 * an outcome is written when a vote resolves and never moves, while a tally is
 * recomputed on every read and drops ballots from people who have since left
 * (docs/adr/044). Put side by side they contradict each other, and on the
 * first seeded patch they did: "Did not carry. 2 for, 1 against."
 */
describe('GovernanceRecord — what it claims', () => {
  const src = source('pages/GovernanceRecord.svelte');

  it('states the outcome without a tally that can disagree with it', () => {
    expect(src).toMatch(/Carried by a vote\./);
    expect(src).toMatch(/Put to a vote and did not carry\./);
    expect(src).not.toMatch(/for, .*against/);
    expect(src).not.toMatch(/e\.approve/);
  });

  it('says an election that settled nothing left the council in place', () => {
    // Holdover removes nobody (docs/adr/051). "Rejected" would read as the
    // community turning somebody down.
    expect(src).toMatch(/Settled nothing\. The council kept serving\./);
  });

  it('names who applied a direct change, since no vote stands behind it', () => {
    expect(src).toMatch(/Applied by \$\{e\.actor\}/);
  });

  it('tells an empty record from a broken one', () => {
    expect(src).toMatch(/Nothing settled yet\./);
    expect(src).toMatch(/Could not load the record/);
  });
});

describe('GovernanceShell — the record is reachable', () => {
  const src = source('components/GovernanceShell.svelte');

  it('sits in the governance nav beside proposals', () => {
    expect(src).toMatch(/label: 'Record', href: `\/patches\/\$\{slug\}\/governance\/record`/);
  });
});
