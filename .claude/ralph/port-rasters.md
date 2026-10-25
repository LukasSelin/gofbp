One round of the raster-port loop. A Ralph loop feeds you this same prompt every
round. What earlier rounds did is in git, in `gh pr list` and in MIGRATION.md, not
in your memory, so start every round by reading those.

The work is the two 🔴 rows the 2026-10-25 re-scope put on MIGRATION.md's
"Concepts still missing" list, in its order: `fwiRaster.r`, then `fbpRaster.r`.

## Each round

1. **Find where the last round stopped.** List the open port PRs:
   `gh pr list --state open --search "head:port/"`. MIGRATION.md is read on the
   newest of those branches, or on `main` when there are none.
   - The newest PR is a draft, or its branch has commits after the PR body's last
     "round" note: that row is mid-flight. Check that branch out and carry on
     with it.
   - Otherwise take the top row on the list that is still 🔴 there.
   - No 🔴 row left there: stop (see "Stopping").
2. **Port that one row** by following `.claude/commands/migration-port.md` in
   full, with these two changes, both for this loop only:
   - Branch `port/<row file without .r>` off the **newest open `port/` branch**,
     or off `main` when there is none. Both rows edit the same generator and the
     same ledger, so stacking keeps the second PR free of conflicts. Use
     `--base` with that same branch when you open the PR.
   - The command's unattended rules apply as written: open the PR, never merge
     it, never commit to `main`, never bump `CFFDRS_VERSION`.
3. **The oracle is the pinned container**, `./testdata/regen-cffdrs.sh --docker`.
   It is never the local Windows R install, whose output differs by ulps. A
   fixture from that install may be used to check that the generator runs, but its
   digest is never recorded.
4. **Leave a trail for the next round.** Before you end the round, add a
   "round N" note to the PR body: what is done, what is not, and the next step.

## The package rule is not yours to change

Both public packages import `math` and nothing else, and `TestPackageHasNoDependencies`
enforces it. The grid entry points therefore loop over plain row-major slices,
sequentially, with no `sync`, no goroutines and no file formats. Parallelism is
the caller's job, and the row says so. If a design seems to need any other
import, that is an "Escalate rather than decide" item. Stop; do not widen the
rule.

## Stopping

Output `<promise>RALPH DONE</promise>`, with one line before it saying which
case applies, when any of these is true:

- both rows have open PRs, neither of them a draft, and `go test ./...` is green
  on the newest one;
- the gate (`go run ./tools/precheck -mode port`) still fails after one
  regeneration attempt;
- the next step is an item on the "Escalate rather than decide" list in
  `.claude/commands/migration-port.md` or `DAILY-CHECK.md`;
- the same test has failed for three rounds in a row with no new idea.

In every other case, end the round without the promise, and the loop runs
another round.
