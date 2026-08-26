<script>
  /**
   * The governance page (CONTEXT.md "Governance page", docs/adr/068): the
   * public explanation of how a patch decides things, reached from About's
   * "How does it work?" section.
   *
   * The second and last explaining surface admitted by docs/adr/068, which
   * amends docs/adr/040's "single standing prose surface". It exists
   * because governance is permission-gated — canSeeGovernance hides every
   * governance surface from non-members — so ADR 040's UI-carries-training
   * mechanism cannot reach the reader who is deciding whether to join.
   *
   * Project-owned and fixed, the lining's model (docs/adr/037) rather than
   * the legal documents' (docs/adr/028): a description of what the
   * software does is the project's claim, and an instance that could edit
   * it could narrate behavior it does not control (docs/adr/049).
   *
   * The spine is the three axes a patch configures — who decides, how a
   * decision is made, who leads — never a tour of features. Feature tours
   * grow a section per release; axes are enumerated in code, which is what
   * lets governance-page.test.js pin this prose to them. Every value in
   * DECISION_OPTIONS (StructuredRulesEditor.svelte) and in the leadership
   * models map (GovernanceOverview.svelte) must be named here or that test
   * fails. Patch governance only: the quilt's own accountability lives on
   * the Label.
   */
  import { navigate } from '../stores/router.svelte.js';
  import { getInstanceName } from '../stores/quilt.svelte.js';

  let instanceName = $derived(getInstanceName());

  function go(path) {
    return (e) => { e.preventDefault(); navigate(path); };
  }
</script>

<svelte:head>
  <title>How governance works &mdash; {instanceName}</title>
</svelte:head>

<div class="governance-page page-fade">
  <header class="gov-header">
    <h1>How governance works</h1>
    <p class="gov-standfirst">
      Every patch runs itself. A band never has to think about any of
      this, and a coalition of forty people can hold real votes with a
      real record. Both are the same software; what differs is how much of
      it a patch switches on.
    </p>
  </header>

  <section class="gov-section">
    <h2>Who decides</h2>
    <p>
      A patch's electorate is its active admins and members, once they
      have been there as long as that patch asks. Followers are not in it.
      Following is meant to be frictionless and costs nothing, so it
      carries no say in what the patch does, and becoming a member is the
      step that changes that.
    </p>
    <p>
      A patch can let its followers read along with proposals and comment
      on them. It cannot give them a vote.
    </p>
  </section>

  <section class="gov-section">
    <h2>How a decision gets made</h2>
    <p>
      Patches sit in one of three places, and they are genuinely
      different.
    </p>
    <p>
      <strong>An admin decides.</strong> The change is applied the moment
      it is made. It still lands in the patch's governance history where
      members can see it, but nobody was asked. Most small patches live
      here and should.
    </p>
    <p>
      <strong>The patch votes.</strong> A proposal opens for voting the
      moment it is raised, and discussion happens alongside the vote
      rather than in a stage before it. When voting opens, the rules the
      vote runs under are fixed for its whole life: who may vote, how many
      of them have to turn out, and what carries it. A patch can change
      its rules while a vote is running, and that vote still finishes
      under the rules it started with. You do not get to use the new rules
      to pass the change to the new rules.
    </p>
    <p>
      What carries a proposal is the patch's own setting: a majority, a
      two-thirds supermajority, or full consensus.
    </p>
    <p>
      <strong>The decision happens somewhere else.</strong> Some
      communities decide at a meeting, on a show of hands, in a room with
      no software in it. A patch can say so, and then record what was
      decided as an attestation: the membership decided this, elsewhere,
      on this date. Patchwork cannot check that claim and does not pretend
      to. It records who asserted what, in public, and a mistaken record
      is corrected by a later one rather than quietly edited.
    </p>
  </section>

  <section class="gov-section">
    <h2>Who leads</h2>
    <p>
      Admins run a patch. How someone becomes one is the patch's third
      choice, and each answer brings its own mechanics.
    </p>
    <p>
      <strong>A maintainer</strong> keeps the patch and names their own
      successor. <strong>Meritocratic</strong> patches follow the open
      source pattern: admins nominate from active members when the patch
      needs another, and the community ratifies. <strong>Elected</strong>
      patches are the only ones with seats and terms, filled by an
      election that comes round on a calendar. An election is an ordinary
      proposal underneath, with the same electorate and the same rules
      fixed at open; the electorate approves as many candidates as it
      likes and the most-approved take the open seats.
    </p>
    <p>
      A term running out never removes anyone. The seat shows publicly
      that it is overdue and its holder keeps serving until a successor is
      elected, because a clock should not be able to leave a community
      leaderless.
    </p>
  </section>

  <section class="gov-section">
    <h2>What every patch starts from</h2>
    <p>
      Whatever it chooses, a patch begins by adopting
      <a href="/lining" onclick={go('/lining')}>the lining</a>, the
      baseline this whole quilt shares. A patch can write its own charters
      on top of it and can amend its copy of the lining itself, and if it
      does, that is public on the patch and it wears a badge saying so.
    </p>
    <p>
      Charters are the community's own words, and they can promise things
      no software enforces. That is legitimate and it is where such
      promises belong. This page only describes what Patchwork itself
      does.
    </p>
    <p class="gov-aside">
      This is about how the patches here govern themselves. For who runs
      this quilt and what it costs to keep on, read
      <a href="/label" onclick={go('/label')}>the Label</a>.
    </p>
  </section>
</div>

<style>
  .governance-page {
    max-width: var(--pw-measure);
    margin: 0 auto;
    padding: 32px 0 64px;
  }

  .gov-header {
    margin-bottom: 32px;
  }

  .gov-header h1 {
    margin: 0 0 12px;
    font-size: 1.5rem;
  }

  .gov-standfirst {
    margin: 0;
    line-height: 1.6;
    color: var(--color-text-muted);
  }

  .gov-section {
    margin-bottom: 28px;
  }

  .gov-section h2 {
    font-size: 1.05rem;
    margin: 0 0 10px;
  }

  .gov-section p {
    margin: 0 0 10px;
    line-height: 1.6;
  }

  .gov-section a {
    color: var(--color-primary);
  }

  .gov-aside {
    margin-top: 16px;
    padding-top: 12px;
    border-top: 1px solid var(--color-border);
    font-size: 0.9rem;
    color: var(--color-text-muted);
  }
</style>
