# Daily migration check

Run this once a day, in order, and leave [MIGRATION.md](MIGRATION.md) more
accurate than you found it. It should take ten minutes on a quiet day.

The point is not to port something every day. It is that on any given morning
the ledger tells the truth about where the port stands — including the days the
answer is "nothing moved". A ledger that is only updated when work happens
cannot be trusted on the days it matters.

**One rule above the rest:** never mark a row ported because the Go code looks
right. A hand-transcribed coefficient that is wrong looks exactly like one that
is right. Only the fixture can tell them apart.

---

## 1. Ground truth (2 min)

- [ ] `git fetch && git status` — clean tree, know what branch you are on.
- [ ] `go test ./...` — green before you touch anything. If it is red, stop; today's job is that.
- [ ] Note how many `TestCFFDRS*` tests **skipped**. All fifteen skipping means you have no fixture, so nothing you conclude today about coefficients is backed by anything.

## 2. Upstream drift (3 min)

Against <https://github.com/cffdrs/cffdrs_r>:

- [ ] Compare `HEAD` to the **Upstream commit last read** pin in `MIGRATION.md`. No change → skip to step 4.
- [ ] Read [`NEWS.md`](https://github.com/cffdrs/cffdrs_r/blob/main/NEWS.md) for every version between the pin and now. This is the cheapest possible signal that a coefficient moved.
- [ ] Read the diff of `R/` only — `https://github.com/cffdrs/cffdrs_r/compare/<pinned-sha>...main`. Ignore `man/`, `inst/`, roxygen churn.
- [ ] For each changed R file, answer in one line: **does this touch a row gofbp claims?**
  - Touches a ✅ or 🟢 row → this is the highest-priority work in the repo. A silently-changed reference number outranks any new feature.
  - Touches a 🔴 row → update that row's note so the eventual port targets the *current* upstream, not the version you first read.
  - Touches a ⚪ row → confirm the out-of-scope reason still holds, then move on.
- [ ] Update the **Pins** table (commit sha, version, date) even when nothing else changed. That date is the whole value of the pin.

## 3. If upstream changed anything gofbp implements (as long as it takes)

- [ ] Bump `CFFDRS_VERSION` in `testdata/Dockerfile`.
- [ ] `./testdata/regen-cffdrs.sh`
- [ ] `go test . -run TestCFFDRS`
- [ ] **Read the diff in the reference numbers.** A changed oracle number is the reference implementation telling you something. It is never noise, and it is never something to commit past.
- [ ] Record the new fixture sha256 in `testdata/README.md`, keeping the previous digest, so a stale local fixture identifies itself instead of failing obscurely.
- [ ] Say in the PR what the reference numbers did and why.

## 4. Ledger sweep (3 min)

Walk `MIGRATION.md` top to bottom:

- [ ] Every ✅ row: does a `TestCFFDRS*` still assert it? A row that quietly lost its oracle coverage is worse than one that never had it — it is a false claim.
- [ ] Every 🟢 row: is there still genuinely no upstream column to assert against, or did upstream start returning one?
- [ ] Every 🟡 row: is the note still an accurate description of the gap?
- [ ] Every 🔴 row: still blocked by what the dependency order says, or did its blocker clear?
- [ ] Every ⚪ row: the reason is the row's whole content. If you cannot restate the reason in a sentence, it is not out of scope — it is unported.
- [ ] Any Go file added or changed since yesterday: is it in the ledger at all?

## 5. Move one thing (the rest of the day)

- [ ] Take the **top unblocked 🔴** from "Concepts still missing, in dependency order". Do not skip down the list because something lower looks easier — the order is a dependency order, and porting out of it produces code the oracle cannot check yet.
- [ ] Port it. Transcribe from the ST-X-3 equations *and* read the R, and note where the R disagrees with the published paper — that disagreement is usually a documented revision, and occasionally it is the bug.
- [ ] Add the fixture column to `testdata/gen_cffdrs_reference.R`, regenerate, and write the `TestCFFDRS*` before you call it done. A port with no oracle column is a 🟢 at best, and only if you can say why no column can exist.

## 6. Close the day (1 min)

- [ ] Update the row's status and the **Pins** table.
- [ ] Add one line to the **Log** table — **including on days nothing moved.** Write `no upstream change; no port` and the date. A gap in the dates should mean the check was skipped, never that it was quiet.
- [ ] Commit the ledger with the work, not separately. `MIGRATION.md` is part of the change, not a report about it.

---

## Escalate rather than decide

Stop and ask before doing any of these. Each one changes what the package
*claims to be*, which is not a daily-check-sized decision:

- Adding a default for FMC, SFC, CBH, CFL or a fuel-type mapping. The absence of
  local judgement is the package's central claim; a default is local judgement.
- Committing the fixture. It is GPL-2 output in an MIT repository, and that is
  the reason it is generated rather than stored.
- Moving a ⚪ row into scope, or a 🔴 row out of it.
- Making the oracle workflow run on push. Each run rebuilds an R and GDAL image
  from scratch.
