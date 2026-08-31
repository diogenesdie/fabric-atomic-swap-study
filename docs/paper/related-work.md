# Related Work and Novelty Positioning

> Draft material for the paper (English). Theoretical notes in Portuguese live
> in [`docs/notas/`](../notas/).

## The gap statement

The claim below survived a systematic search of the literature (August 2026).
It is deliberately narrow — every qualifier is load-bearing and each one
excludes a specific piece of prior work.

> To the best of our knowledge, no prior work provides a **controlled,
> implementation-based empirical comparison** of HTLC and 2PC-style atomic
> commitment for cross-chain transactions on a **permissioned platform**
> (Hyperledger Fabric 2.5) under **systematic fault injection**. Prior
> comparisons are either analytical [Engel et al. 2021], model-based and
> permissionless [Zakhary et al. 2020], or evaluate a single newly proposed
> protocol rather than contrasting protocol families [Tao et al. 2024;
> Lu et al. 2024].

Drop any qualifier and the claim becomes false. Keep them and it holds.

## Five works that must be explicitly differentiated

A reviewer will raise at least two of these. Each needs one or two sentences in
Related Work, not a dismissal.

### 1. Engel, Herlihy & Xue (SSS 2021) — the conceptual predecessor

`engel2021failure`. Traces the evolution of cross-chain protocols "starting with
two-phase commit and finishing with timed hashlocked smart contracts", and
reframes HTLC abort as an *option* rather than a failure. This is our comparison,
done analytically.

**Differentiation:** it is an invited conceptual paper with no implementation and
no measurement. We quantify on a deployed platform what they argue structurally.
Their optionality framing is genuinely useful and we should adopt it when
interpreting H2/H3 results — an HTLC abort after secret revelation is not
symmetric with a 2PC abort, and their vocabulary says why.

### 2. Zakhary, Agrawal & El Abbadi (PVLDB 2020) — the empirical threat

`zakhary2020atomic`. AC3WN's evaluation already compares latency and throughput
against state-of-the-art HTLC atomic swaps.

**Differentiation:** three axes. Their comparison is (a) largely analytical and
model-based, with permissionless block times (Bitcoin/Ethereum) dominating every
measurement; (b) not on a permissioned platform; (c) **without controlled fault
injection** — the very failure mode they use to argue against HTLC (expired
timelock under delay) is asserted, not measured. Our H3 scenario tests their
central claim empirically. This is the strongest framing available to us: we are
not contradicting AC3WN, we are supplying the experiment its argument presumes.

### 3. Tao, Li & Li (IEEE TKDE 2024) — closest in spirit

`tao2024atomicity`. Atomicity across *permissioned* blockchains *under failures*,
proposing the Unity protocol (4PC) with experimental evaluation.

**Differentiation:** they propose and benchmark a new protocol against their own
baselines; they do not perform a controlled comparison between protocol families,
and their second axis is confidentiality rather than a fault taxonomy.
**Action item: read in full before writing the introduction** — this is the paper
most likely to already contain a result we think is ours.

### 4. Lu, Jajoo & Namjoshi (ACM DIN 2024 + arXiv extended)

`lu2024twophase`, `lu2024atomicity`. Two-phase protocol for atomic multi-chain
transactions, prototyped on EVM testnets connected by LayerZero bridges.

**Differentiation:** Solidity on permissionless testnets, no HTLC baseline, no
fault injection. Their reported LOC breakdown (a 225-line atomicity executor
within ~1,068 lines total) independently validates our effort estimate for the
2PC arm — worth citing as such. Cite both versions; the arXiv one carries the
technical detail.

### 5. `leoloco/Two-Phase-Commit-Atomic-Cross-Chain-Swap` — grey literature

A public repository combining HTLC contracts with blockchain relays and adapters,
apparently a master's thesis artifact. Pseudocode only, no published evaluation.

**Differentiation:** no threat to novelty, but a footnote is the honest move.

## Two external supports for the gap claim

1. The 2025 PRISMA systematic review of permissioned interoperability
   (`alsoudi2025permissioned`) reports that **ACID properties and cross-chain
   resilience are underexplored**, and that 93% of surveyed studies examine fewer
   than four properties. Third-party evidence that the gap is real, not
   self-serving.

2. `yan2025empirical` (ASIA CCS 2025) shows that empirical cross-chain
   work exists but targets deployed public bridges — a measurement study of the
   wild, not a controlled protocol comparison. Different object, same adjective.

## An additional original angle

HTLC on Fabric is **not native**. Fabric has no built-in time management, so a
timelock is enforced by comparing the claim transaction's timestamp against a
peer's local clock, assuming approximate clock synchronisation. That assumption
is itself an injectable variable, and no prior work appears to have measured
what clock skew does to HTLC safety on a permissioned ledger. Scenario X1.

If the core matrix completes on schedule, this is the highest-value extension:
it is cheap to run, nobody has done it, and it strikes directly at the synchrony
hypothesis that the whole HTLC argument rests on.

## Structural note for the paper

The comparison is **asymmetric in engineering effort**, and we should say so
rather than hide it: the HTLC arm is reused wholesale from a maintained
production codebase (Hyperledger Cacti/Weaver), while the 2PC arm had to be
implemented because no cross-chain 2PC exists for open-source Fabric. That
asymmetry is itself a finding about the state of the ecosystem — and it is worth
noting that Oracle Blockchain Platform added proprietary cross-channel 2PC
precisely because upstream Fabric lacks it, marketing it as working "without
needing HTLCs".

The 2PC arm's message vocabulary follows IETF SATP (`ietf2026satp`), whose
Stage 3 is literally a two-phase commit between gateways. Aligning the naming
costs nothing and pre-empts the objection that our 2PC is a straw man of our own
design.
