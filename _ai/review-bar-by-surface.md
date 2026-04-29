# Review bar varies by surface: the front-page bar is not the code-review bar

A "non-blocking" verdict from Kevlin (low-level) or Linus (high-level) on a cosmetic README nit can still be **upgraded to blocking by Don** when the affected surface is the front page of the repo. The reviewer-tier severity ("does it parse", "are the load-bearing claims correct") is the right bar for code; the **front-page bar is "would I be embarrassed if a peer saw this"**, which is strictly stricter than either reviewer's default bar.

## The override

`_tasks/2026-04-29-readme-and-license/`: Kevlin (`007-kevlin-review.md` §1.10a) and Linus (silent on it) flagged a port-punctuation inconsistency at `README.md:13` (`80/443/443-UDP`) vs `README.md:76` (`` `80/tcp`, `443/tcp`, and `443/udp` ``) as a non-blocking cosmetic nit. Don overrode both in `008-don-final.md` §2.2 and called it blocking.

His defense:

> "Kevlin's bar ('does it parse') is the right bar for code review. Linus's bar ('ship if all the load-bearing claims are correct') is the right bar for high-level review. Neither is the right bar for the front page of the repo. The front-page bar is 'would I be embarrassed if Brendan Eich saw this?' The answer with line 13 as written is 'mildly, on a slow afternoon.' The answer with line 13 fixed is 'no.' That's the difference worth one round-trip."

Three words, one round-trip, two reviewers' "non-blocking" upgraded to blocking. Rob applied the fix in commit `71674c3`; Linus and Joel confirmed in 30 seconds; squash-merge proceeded.

## When to apply

The upgrade rule applies when ALL of:

1. **The surface is reader-facing and high-traffic** — README, top-level LICENSE, project landing page. Not internal docs, not test code, not implementation comments.
2. **The fix is genuinely cheap** — Don's threshold was three words. The point is not to relitigate well-defended choices (the Architecture title, the "One backup path" sentence — both stayed) but to fix the small inconsistency the reviewers correctly identified and correctly graded as cosmetic.
3. **The cost of NOT fixing compounds** — every reader of the front page sees the inconsistency; every drive-by edit risks "fix as scope creep" rather than "fix while we're here." Fix once, fix in scope.

The upgrade does NOT apply to:

- Subjective wording the reviewers explicitly defended (Architecture title, "One backup path" sentence — both got considered-and-kept defenses from Rob, Kevlin, AND Linus, which is when the tech lead defers).
- Code-review nits where "does it parse" or "is it correct" really is the right bar.
- Anything where the round-trip cost exceeds the fix value (a 30-minute fix with a one-week review queue is not worth one more PLAN re-entry; three words with two 30-second confirmations is).

## Why this isn't bikeshedding

The discipline distinguishes itself from bikeshedding by being **single-pass and bounded**. Don made one call, gave a verbatim diff (`008-don-final.md` §2.3), and Rob applied exactly that diff in one commit (`71674c3` — `1 file changed, 1 insertion(+), 1 deletion(-)`). No back-and-forth on alternatives. No reopening of other defended decisions. The override exists to break ties on cosmetic-but-conspicuous front-page items; using it to relitigate substantive decisions would be the failure mode.

Linus's confirmation pass (`009-linus-final.md`) was three checks, three pass, four sentences each. That cost-shape is what justifies the override; if it had triggered another full review cycle, the round-trip would have been more expensive than the lapse.

## Originator

`_tasks/2026-04-29-readme-and-license/008-don-final.md` §2.2, §7 (Don's defense of overriding both reviewers); `009-linus-final.md` (the 30-second confirmation pass that validated the round-trip cost shape).
