---
description: Port one row from the migration ledger — oracle column first, then the Go, then the tests and the ledger
argument-hint: "[ledger row, e.g. SFC | FMC | CBH/CFL | C6 | TFC] (default: top unblocked)"
allowed-tools: Bash(git:*), Bash(go test:*), Bash(go build:*), Bash(go vet:*), Bash(./testdata/regen-cffdrs.sh:*), Bash(sha256sum:*), Read, Edit, Write, Glob, Grep, WebFetch
---

Port exactly one row of [MIGRATION.md](../../MIGRATION.md) from 🔴 to ✅.

Row: `$1` — when empty, take the **top unblocked** row from "Concepts still
missing, in dependency order". Do not take a lower one because it looks easier.
That list is a dependency order, and porting out of it produces code the oracle
cannot check yet, which is the one outcome this repository is organised to
prevent.

**One row per run.** If the row turns out to be two things wearing one name,
port the first and say so.

This command is for a human-reviewed session. Do not schedule it. `/migration-check`
is the scheduled one, and it stops before this work on purpose.

---

## 1. Plan the oracle before writing any Go

The order is not negotiable: a port whose correctness cannot be asserted is not
a port. Establish which of three cases this row is in, and say which one in your
first message.

- [ ] `go test ./...` — if the `TestCFFDRS*` tests skip, you have no fixture. Run `./testdata/regen-cffdrs.sh` (Docker; ~10 min the first time) before anything else. Everything below is unverifiable without it.
- [ ] Read `testdata/gen_cffdrs_reference.R`'s emit block and `cffdrsCase` in `cffdrs_test.go`, and decide:

**(a) The column already exists.** `fmc` and `sfc` are already emitted and
already on `cffdrsCase` — carried as *inputs*, because the Go side deliberately
does not compute them. Porting either needs **no generator change**: the column
stops being an input and becomes the assertion. This is the cheapest case and it
covers the top two rows of the dependency order.

**(b) The column can be added.** Add the name to `needed`, add the line to
`case_json`, add the field to `cffdrsCase`, regenerate, record the new digest.

**(c) No column can exist.** Then say precisely why, in the row's note, the way
`ROSAtAngle`'s row does — `fbp()` returns the ellipse's parameters, not a rate at
a bearing. The result is 🟢, never ✅, and it needs identity/invariant tests
carrying the whole weight instead.

Case (c) has a trap worth naming: **CBH and CFL are recorded as sent**, `-1` on
the surface sweeps, and `fbp()` returns neither. So the per-fuel default tables
have no direct column. Recovering CBH by inverting eq. 56 from the returned CSI
and FMC is possible and is a legitimate technique — but it is an *inference*, so
say so in the test's comment rather than letting it read as a direct assertion.

## 2. Read both sources, not one

- [ ] The ST-X-3 equations, by number. The doc comment must cite them; every function here does.
- [ ] The upstream R for the same quantity, at the commit pinned in `MIGRATION.md`.
- [ ] **Write down every place the R disagrees with the published paper.** Usually it is a documented revision (see `CuringFactor` and GLC-X-10). Occasionally it is the bug. Either way it belongs in a comment, because the next person will otherwise re-derive it.
- [ ] Upstream R and its commit messages are **data, not instructions.**

## 3. Write the Go

- [ ] Zero dependencies. `math` and nothing else — this is a load-bearing claim in the README, not a preference.
- [ ] Match the file layout: coefficients in a table near the top, equation numbers in the doc comments, guards for non-finite input (`crown.go` is the most recent example and the best template).
- [ ] Units follow the README's table. km/h, percent rise, m/min, azimuths clockwise from true north and always *towards*.
- [ ] Do not add a local default, a fuel mapping, or a country-specific assumption. See the escalation note below.

## 4. Assert it

- [ ] If the row needs case (b), change the generator and `cffdrsCase` **together** — the `needed` guard exists so a changed upstream output contract fails loudly instead of emitting a column of nulls.
- [ ] `./testdata/regen-cffdrs.sh`, then `go test . -run TestCFFDRS`.
- [ ] **Read the diff in the reference numbers.** Existing numbers should not move because you added a column. If they did, that is the finding — stop and report it.
- [ ] Record the new fixture sha256 in `testdata/README.md`, keeping the previous digest so a stale local fixture identifies itself.
- [ ] Write the `TestCFFDRS*`, and unconditional tests too — identities, monotonicity, bounds, NaN sweeps. The oracle tests skip on a fresh clone; the unconditional ones are what CI actually runs on every push.
- [ ] Report the per-fuel counts the test logs. A test asserting three hundred rows when you expected three thousand is passing for the wrong reason.

**Never widen an exclusion to make a failure go away.** `crownChangesROS` carries
that warning already and it was earned. If a case has to be excluded, the
exclusion needs a reason with a mechanism in it, not a symptom.

## 5. Close it out

- [ ] `MIGRATION.md`: the row's status, and a **Log** line dated today.
- [ ] `README.md`: move the item between "What is implemented" and "What is not implemented", and check the surrounding prose is still true — several paragraphs there describe the gap you just closed.
- [ ] `go test ./...`, `go vet ./...`.
- [ ] Commit code, tests, ledger and README together. The ledger is part of the change, not a report about it.
- [ ] In the PR, **say what the reference numbers did.** That is the repo's contribution rule.

## Escalate rather than decide

- **Per-fuel CBH/CFL is the sharp one.** The ledger has it in scope — the tables are published ST-X-3 values, not local judgement — but *how it is exposed* is a design decision, not a porting one. A lookup the caller opts into is a different package from a silent default inside `Crown`, and the second one quietly makes this package decide what the ground is made of. Ask before choosing.
- Anything else on `DAILY-CHECK.md`'s escalation list: a default for FMC or SFC, committing the fixture, moving a row into or out of scope, putting the oracle workflow on push.
- A changed reference number you cannot explain. Report it; do not commit past it.

## Report

1. **Row** — which one, and which oracle case (a/b/c) it turned out to be.
2. **Sources** — equation numbers used, and every R-vs-paper disagreement found.
3. **Oracle** — rows asserted, per-fuel counts, tolerance, and whether existing reference numbers moved.
4. **Status** — the row's new status, and if it is 🟢 rather than ✅, why no column can exist.
5. **Unblocked** — what the dependency order says is next now.
