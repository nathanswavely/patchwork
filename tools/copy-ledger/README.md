# Copy ledger

Who wrote the words a visitor reads.

Much of Patchwork was built with AI assistance, and much of its copy was
drafted that way too. This tool exists to replace those drafts with a
person's writing, and — the harder half — to keep them replaced.

The ledger records one decision per string. It is checked in, because the
ledger **is** the transparency claim: a file anyone can diff beats a
paragraph asserting the copy is human. Where a draft was replaced, both
texts are kept.

## The loop

```sh
make copy-sync      # read the source, fold new strings into the ledger
make copy-review    # write, at localhost:5175
make copy-apply     # dry run: show what would change in source
make copy-apply APPLY=1
make copy-stats     # progress
make copy-report    # render copy/REPORT.md for publishing
```

## Reviewing without a server

The UI needs a machine to serve it and a browser that can reach that
machine, which rules out most of the places writing actually happens. So
the queue also comes out as Markdown:

```sh
make copy-draft FILE=README.md   # → copy/drafts/README.md.md
#  …edit it anywhere: GitHub's web editor, a laptop, a phone…
make copy-pull                   # read your writing back into the ledger
make copy-apply APPLY=1          # write it into source
```

Each string sits between `<!-- copy:ID -->` markers. Edit the text
between them; leave a block alone and it's skipped, so three strings and
come back is a fine way to work. Replace a whole block with `@mine` if
the words are already yours, or `@fine` to leave a draft in place on
purpose.

`FILE=` scopes it to one source file, which is the difference between a
page of work and all 1,853 strings at once. `copy-draft` refuses to
overwrite a draft holding writing you haven't pulled yet, and `copy-pull`
deletes draft files once every string in them is decided.

If you edit drafts on the branch in GitHub, the commit is **yours** —
which makes `git blame` a second, independent record of who wrote the
copy. The ledger says so; the history proves it.

## Bulk decisions

For a file whose copy has one answer, decide it in bulk instead:

```sh
node tools/copy-ledger/cli.js decide --status human \
  --file internal/governance/lining.go --note "why" --apply
```

It only moves entries out of `unreviewed` (unless `--force`), so re-running
can't quietly undo work done string-by-string in the UI. Dry run by default.

`make copy-check` is the CI gate. It runs on every PR.

## The four decisions

Every string is one of:

| Status | Means |
|---|---|
| `unreviewed` | A model drafted it and nobody has looked. The default. |
| `rewritten` | You wrote a replacement; `copy-apply` hasn't written it yet. |
| `human` | The text in source is a person's words. |
| `ai-fine` | Deliberately left as drafted — mechanical, not voice-bearing. |

`ai-fine` is not a loophole, it's the honest option for "Failed to load
comments". Spending a Saturday rewriting error toasts buys nothing; saying
so out loud costs nothing. The report counts them separately from `human`,
so the published number never overstates the case.

## The ratchet

This is the point of the tool. Rewriting 1,869 strings by hand is a finite
job; keeping them rewritten is not, because the next AI-assisted session
adds forty more and the claim quietly stops being true.

So `copy-check` fails CI when a string enters the source with no decision
recorded. The existing backlog is grandfathered — `ledger.baseline` holds
the IDs accepted as debt on day one — so the gate blocks **new** copy only.
It is not a wall in front of the cleanup.

When the backlog reaches zero, set `"strict": true` in the ledger and the
gate starts failing on any unreviewed string at all.

## Review UI

`make copy-review` serves a local page on `127.0.0.1:5175`. It is not part
of the Patchwork binary, not embedded, not served by the app — a workbench
that runs on your machine and ships to nobody.

Paragraphs sort first: 266 of them carry most of the voice, against 1,190
two-word labels that mostly need a glance.

| Key | |
|---|---|
| `j` / `k` | next / previous |
| `e` | edit |
| `⌘↵` | save your version |
| `h` | already my words |
| `a` | leave as drafted |
| `⌘K` | search |

It warns if your rewrite drops a `{placeholder}` the original interpolated.

## How writeback stays safe

Every occurrence stores the **verbatim** source substring it came from.
Writeback finds that exact substring or reports drift and changes nothing —
no fuzzy matching, no offset guessing. If one of an entry's nine
occurrences has drifted, none of the nine are touched: a sentence that
reads differently in nine places is worse than one nobody has rewritten
yet.

It also refuses text that would break the syntax it lands in — a `{` in
Svelte markup, a quote that would close a Go string literal, a newline in
an HTML attribute — and tells you which entry and why, rather than
producing a file that doesn't compile.

Writeback edits source files. Run it on a clean tree and read the diff.

## Scope

In: Svelte markup and copy-bearing attributes (`placeholder`, `title`,
`aria-label`, `alt`), sentence-shaped strings in Svelte/JS, prose in the Go
files that ship text (the lining, legal defaults, governance templates,
notification and email bodies), and the Markdown a newcomer actually reads.

Out, on purpose:

- **`docs/adr/`** — ~50k words of decision records. Engineering reasoning,
  not the project's voice. Drafted with AI assistance and left that way.
- **`CODE_OF_CONDUCT.md`** — the Contributor Covenant. Rewriting a standard
  text to sound like us would defeat the point of adopting a standard text.
- **Embedded JSON** in `defaults.go` — `governance-rules.json` templates are
  machine configuration, which this project already treats as a different
  kind of thing (docs/adr/053).
- **Frozen constants** (`FROZEN_GO_CONSTS` in `scope.js`) — the retired
  lining drafts. They are AI-drafted, and they stay that way on purpose:
  `AutoUpdateLinings` identifies a patch's lining by matching these exact
  bytes to decide whether it is stale and should heal (docs/adr/037). Editing
  one character strands every patch still carrying that text — it stops
  healing and wears an "Amended lining" badge it never earned. They are kept
  out of the queue entirely rather than trusted to a `note`, because you
  cannot mistakenly rewrite what the tool never offers you.
- Source comments, tests, commit messages.

`copy-report` prints these exclusions alongside the numbers. A coverage
claim that hides its own scope is the thing this tool is meant to avoid.

Scope lives in `scope.js` and is a policy decision, not a technical one.

## False positives

The string filters over-collect on purpose. A false positive costs one
keystroke to mark `ai-fine` forever; a false negative is copy that silently
never gets reviewed. If you see something odd in the queue, that's the
trade working as intended.
