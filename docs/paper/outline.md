# Paper outline

Working title: **Atomicity Under Failure: An Empirical Comparison of HTLC and
Two-Phase Commit for Cross-Chain Transactions on Hyperledger Fabric**

Target length: 8–10 pages. Numbers below are medians from
`docs/paper/figures/results.md`; regenerate with `make analyze` before writing.

---

## 1. Introduction

Open with the gap statement, which every qualifier earns:

> To the best of our knowledge, no prior work provides a controlled,
> implementation-based empirical comparison of HTLC and 2PC-style atomic
> commitment for cross-chain transactions on a permissioned platform under
> systematic fault injection.

Then the contribution, in the order a reader wants it:

1. Both protocol families implemented and measured on the same platform, under
   the same injected faults, with outcome classified by reading both ledgers
   rather than trusting either client.
2. **The headline result: both families can break atomicity, for the same
   underlying reason.** The mechanism each uses to buy liveness back — the
   timelock in HTLC, the unilateral timeout abort in 2PC — is what costs it
   safety. This contradicts the expectation we started with, and it is the
   paper's reason to exist.
3. A quantified cost model: what each protocol charges, in blocked capital and
   in latency, for the safety it does provide.
4. A reproducible artifact: one command brings up two networks and runs the
   matrix.

State the negative results too — they are cheap to report and expensive to
rediscover:

- Uniform network delay does **not** break HTLC atomicity on Fabric. The
  timelock is evaluated against the transaction *proposal* timestamp, so a claim
  submitted before expiry remains valid however late it commits.
- Client clock skew costs liveness, not safety, because the deadline is an
  absolute value both parties read from the ledger.

## 2. Background

Keep it tight; the reviewers know this. Two things must land:

**Atomic commitment is not consensus.** Consensus decides by quorum; atomic
commitment requires unanimity to commit, so a single refusal aborts everything.
Cite Gray (1978) for 2PC and the blocking problem, Skeen (1981) for the
non-blocking impossibility under partition, Gray & Lamport (2006) for Paxos
Commit as the replicated-coordinator answer.

**HTLC replaces the coordinator with cryptography and time.** Nolan (2013),
formalised by Herlihy (2018). The hashlock propagates the decision — revealing
the preimage to claim on one chain enables the counterparty on the other. The
asymmetric timelocks (T1 > T2) protect whoever reveals first.

Then the framing sentence for the whole paper: *both protocols trade the same
way, and the experiment is about measuring the exchange rate.*

## 3. Related work

Four works need explicit differentiation. One or two sentences each; a reviewer
will raise at least two of them.

| Work | What it does | Our difference |
|---|---|---|
| Engel, Herlihy & Xue (SSS 2021) | Traces 2PC → HTLC, reframes abort as optionality | Analytical, no implementation, no measurement |
| Zakhary et al. (PVLDB 2020) | AC3WN; compares latency against HTLC | Model-based, permissionless, **no fault injection** — the failure their argument rests on is asserted, not measured |
| Tao, Li & Li (TKDE 2024) | Atomicity across permissioned chains under failures | Proposes a new protocol (4PC); does not contrast protocol families. **Read in full before submitting** |
| Lu, Jajoo & Namjoshi (DIN 2024) | Two-phase multi-chain protocol | Solidity over LayerZero, no HTLC baseline, no faults |

External support for the gap: the 2025 PRISMA review of permissioned
interoperability reports ACID properties and cross-chain resilience as
underexplored, with 93% of surveyed studies examining fewer than four
properties.

Also note, honestly, that Engel et al. anticipated the *conceptual* shape of our
headline result. Our contribution is that it happens, on a real platform, and
what it costs.

## 4. System design

### 4.1 Platform

Two independent Hyperledger Fabric 2.5.16 networks, each with its own peer,
orderer and CA. Not two channels: channels share an ordering service, which
would make independent failure injection impossible and weaken the cross-chain
framing.

### 4.2 The HTLC arm

Reused wholesale from Hyperledger Cacti/Weaver (Apache-2.0): the
`assetexchange` library, the `simpleasset` chaincode, and the Go SDK. Our
contribution here is the instrumented orchestrator that drives the six steps
with controlled crash points.

### 4.3 The 2PC arm

Implemented by us — **no cross-chain 2PC exists for open-source Fabric**. State
that plainly: neither AC3WN nor Lu et al. released code, and Oracle's
Blockchain Platform added proprietary cross-channel 2PC precisely because
upstream Fabric lacks it. The asymmetry in engineering effort between the two
arms is itself a finding about the ecosystem.

Message vocabulary follows IETF SATP (`draft-ietf-satp-core`), whose Stage 3 is
a two-phase commit between gateways. This pre-empts the objection that we
compare against a straw man of our own design.

Three Fabric constraints shaped the design and are worth a paragraph each,
because they generalise to anyone building this:

1. **`InvokeChaincode` across channels is read-only** — its result never enters
   the write set. No transaction spans both chains, so the coordinator *must*
   be an external process. Convenient for us: an external process is easy to
   kill at a chosen point.
2. **No timers in chaincode.** `time.Now()` breaks endorsement determinism.
   Deadlines use `GetTxTimestamp()` and nothing fires on its own — every
   timeout path needs an external watchdog. An expired lock stays locked until
   someone invokes the release.
3. **Latency is dominated by block cutting**, not protocol logic
   (`BatchTimeout: 2s`). What distinguishes the protocols is the number of
   writes on the critical path and whether they can be parallelised.

## 5. Experimental methodology

Follow the structure of Thakkar et al. (MASCOTS 2018); anchor the fault
injection in Hajdu et al. (IEEE Access 2020) and Sondhi et al. (DASC 2021).

**Outcome classification.** Every run is classified by reading *both* ledgers
before and after, never by trusting a client's report:

| Outcome | Meaning |
|---|---|
| `COMMITTED_BOTH` | both sides committed — correct |
| `ABORTED_BOTH` | neither committed — correct |
| `VIOLATED` | only one side committed — **safety failure** |
| `BLOCKED` | assets held in escrow with no resolution — **liveness failure** |

**Metrics.** Protocol latency (wall-clock window over the protocol steps, *not*
the sum of step durations — that would double-count 2PC's parallel phases);
blocked-capital duration; write count. Definitions follow the Hyperledger PSWG
metrics white paper.

**Fault injection.** Network delay via `tc`/`netem` from a sidecar sharing the
target's network namespace, since Fabric images carry no `iproute2`. Coordinator
and participant crashes are deterministic stop points inside the client binary,
which is more reproducible than an external signal and equivalent for a
fail-stop model — say so explicitly rather than let a reviewer wonder.

**Scenario matrix.** Thirteen scenarios, N repetitions each.

## 6. Results

### 6.1 The cost of atomicity when nothing fails

B1 (HTLC) against B2 (2PC, parallel phases) and B3 (2PC, sequential phases).
Same four writes in all three; the difference is entirely block rounds.

**B3 is the control that matters.** Sequential 2PC costs the same as HTLC, so
the 2PC advantage comes from parallelising phases, not from the protocol. HTLC
cannot parallelise: step 6 depends on the secret step 5 reveals.

Figure: `latency.pdf`.

### 6.2 Where each protocol breaks

Figure: `outcomes.pdf`.

- **H2** — HTLC violates when the secret is public and the counterparty does not
  claim before the deadline. This is Zakhary's critique, reproduced.
- **T2** — 2PC blocks when the coordinator dies between phases. **T2r** shows
  the write-ahead log recovering it: the decision is persisted with `fsync`
  before the first Commit leaves, so recovery honours the decision that was
  actually taken.
- **T4** — 2PC *violates* when the unilateral timeout fires on only one
  participant. The coordinator recovers, honours its durable commit decision,
  commits one side, and the other refuses because it already aborted.

**This is the paper's core argument.** T4 is not a bug in our implementation;
it is what the escape valve costs. Remove `TimeoutAbort` and 2PC never violates
— it blocks forever instead. Keep it and 2PC inherits exactly the synchrony
dependence that HTLC is criticised for. Connect explicitly to Skeen (1981).

### 6.3 Same fault, different price

H3 against T3: identical 800 ms delay. HTLC loses the margin between deadlines
and a correct client refuses to lock rather than expose itself — the swap aborts
safely, at the cost of blocked capital. 2PC merely slows down and commits.

Under delay, then: **HTLC pays in liveness, 2PC pays in performance.** Neither
violates atomicity from slow network alone.

### 6.4 Clock skew

X1 and X2. A participant whose clock runs fast becomes over-cautious and refuses
safe swaps; one whose clock runs slow proceeds on a margin narrower than it
believes. Neither produced a safety failure, because the deadline is an absolute
value both parties read from the ledger.

A negative result, and worth stating: the synchrony assumption that HTLC depends
on is about *message delay and block production*, not about participants
agreeing on the time of day.

Figure: `blocking.pdf`.

## 7. Threats to validity

Be specific; vagueness here reads as evasion.

- **Both networks run on one host.** Delay is injected, not natural. Absolute
  latencies are dominated by `BatchTimeout` and are not portable; the *ratios*
  between arms are the transferable result.
- **Single organisation per network, one peer each.** No endorsement-policy
  complexity, no MVCC contention from concurrent load. Concurrency is a
  dimension we did not explore.
- **Crashes are stop points in the client, not signals.** Equivalent under a
  fail-stop model; not equivalent under partial failure or partition.
- **The 2PC arm is ours.** A different implementation might block or violate at
  different rates. The SATP-aligned vocabulary and the durable decision are what
  make it a fair representative, but it is one design, not the family.
- **N repetitions of a deterministic scenario.** The variance we report is
  platform noise, not protocol randomness. Frequencies in the outcome figure
  reflect the composition of the matrix, not natural rates of failure.
- **Weaver's HTLC is one implementation of the family**, with its own choices
  (notably evaluating timelocks against proposal timestamps).

## 8. Conclusion

The expected story was that HTLC trades safety for independence while 2PC trades
liveness for a coordinator. What we measured is narrower and more useful: both
protocols hold atomicity while their timing assumptions hold, and both break it
in the same way when those assumptions fail. The choice between them is not
safety versus liveness but *which* liveness cost is acceptable — indefinite
blocking, or the risk that a timeout fires unevenly.

Future work: concurrency and contention; a replicated coordinator (Paxos Commit)
as a third arm; and the effect of `BatchTimeout` on the crossover point.

---

## Writing checklist

- [ ] Regenerate `results.md` and figures from the final N ≥ 10 run
- [ ] Read Tao et al. (TKDE 2024) in full before the introduction
- [ ] Verify every citation against Crossref — the bibliography had fabricated
      authors once (see the header of `refs.bib`)
- [ ] State N and the environment in every table caption (PSWG requirement)
- [ ] Artifact availability statement pointing at the repository and the pinned
      Cacti commit
