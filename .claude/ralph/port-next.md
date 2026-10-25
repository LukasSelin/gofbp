One round of the port loop. A Ralph loop feeds you this same prompt every round.
What earlier rounds did is in git, in `gh pr list` and in MIGRATION.md, not in your
memory, so start every round by reading those.

The work is whatever MIGRATION.md's "Concepts still missing, in dependency order"
lists, taken top first. This prompt names no rows on purpose. The ledger is the
queue, and a row comes onto it only when a human re-scopes it or `/migration-check`
finds upstream drift.

## Each round

1. **Find where the last round stopped.** List the open port PRs:
   `gh pr list --state open --search "head:port/"`. Read MIGRATION.md on the
   newest of those branches, or on `origin/main` when there are none.
   - The newest PR is a draft, or its branch has commits after the PR body's last
     "round" note: that row is mid-flight. Check that branch out and carry on
     with it.
   - Otherwise take the **top** item on the list that is still 🔴 or 🟡 there,
     and not a lower one because it looks easier. That list is the order.
   - The list says "Nothing is owed.", or every item on it already has an open
     PR: stop (see "Stopping").
2. **Port that one row** by following `.claude/commands/migration-port.md` in
   full, with these two changes, both for this loop only:
   - Branch `port/<row file without .r>` off the **newest open `port/` branch**,
     or off `origin/main` when there is none. Rows share the generator and the
     ledger, so stacking keeps later PRs free of conflicts and lets this round see
     the earlier ones' work. Pass that same branch as `--base` when you open the PR.
   - The command's unattended rules apply as written: open the PR, never merge
     it, never commit to `main`, never bump `CFFDRS_VERSION`.
3. **Read the row's note and the list item before writing anything.** Earlier
   ports leave conventions there (slice layout, error types, which oracle case
   applies), and the next row is expected to follow them.
4. **The oracle is the pinned container**, `./testdata/regen-cffdrs.sh --docker`.
   It is never the local Windows R install, whose output differs by ulps. A
   fixture from that install may be used to check that the generator runs, but its
   digest is never recorded.
5. **Leave a trail for the next round.** Before you end the round, add a
   "round N" note to the PR body: the row, what is done, what is not, and the
   next step.

## Not yours to decide

- **Scope.** Never move a row into or out of scope, and never pick a ⚪, 🟢 or ✅
  row to have something to do.
- **The package rule.** Both public packages import `math` and nothing else, and
  `TestPackageHasNoDependencies` enforces it. If a design seems to need any
  other import, stop; do not widen the rule.
- Anything else on the "Escalate rather than decide" lists in
  `.claude/commands/migration-port.md` and `DAILY-CHECK.md`.

## Stopping

Output `<promise>RALPH DONE</promise>`, with one line before it saying which
case applies, when any of these is true:

- there is no row to take: the list says "Nothing is owed.", or every item on it
  has an open PR that is not a draft and `go test ./...` is green on the newest one;
- the gate (`go run ./tools/precheck -mode port`) still fails after one
  regeneration attempt;
- the next step is a decision from "Not yours to decide";
- the same test has failed for three rounds in a row with no new idea.

In every other case, end the round without the promise, and the loop runs
another round.
