# Alert-loss trio (H1 / H2 / H3)

**Status:** all fixed (2026-06-15) — H3 via consumer redelivery caps; H1 + H2 via a transactional outbox with hybrid (inline + relay) publishing
**Source:** [2026-06-15 progress audit](../audits/2026-06-15-progress-audit.md)
**Theme:** three ways a genuinely-new posting's alert can be **silently dropped or stalled**. None corrupt data — Postgres stays correct — they are *alert-delivery* reliability gaps. They matter because the headline claim is "sub-minute detection-to-alert," and they are exactly what an interviewer probes when you say "event-driven, at-least-once delivery."

The pipeline:

```
collector → NATS(SIGNALS) → processor → [1. write Postgres  2. publish events.* to NATS] → dispatcher → Telegram
```

Findings H1 and H2 are two faces of the same **dual-write problem**: the processor writes to Postgres *and* publishes to NATS, and those two systems can't be updated atomically. H3 is independent (consumer-side resilience).

> Severity in practice today: **low** — single user, NATS healthy on localhost, so none fire in normal operation. They become real on a deployed VM where NATS can hiccup, and they are the robustness story behind the resume claim.

---

## H2 — publish-after-commit (primary) — FIXED

**Fixed (transactional outbox, hybrid publish):** the event is now staged into an `event_outbox` table *inside the same transaction* as the posting/event writes (migration `000007`). After commit it is published **inline** (happy path stays sub-second), and on success the row is marked published. A relay goroutine (`RelayOutbox`, every 2s) resends any rows a failed inline publish left behind. So a NATS hiccup at commit time can no longer permanently drop an alert, and the 607ms detect-to-alert latency is preserved. Verified by `TestProcessorStateMachine` (outbox row published inline) and `TestProcessorOutboxRelay` (straggler resent exactly once, no duplicate).

**Original mechanism (for the record):** the DB transaction committed the `postings` + `events` rows first; then `p.publish` sent to NATS and, on failure, only logged. If NATS was briefly unavailable, the event was permanently in Postgres but **never enqueued**, so no alert. On the next poll the same posting re-arrived, its content hash matched, and processing took the `default:` "unchanged" branch ([processor.go:163](../../internal/processor/processor.go#L163)) — so it **never re-emitted**. Alert lost for good, nothing looked broken.

**Why the obvious fix didn't work:** "return an error so NATS re-delivers the signal" fails because the transaction already committed — on redelivery the posting exists and is unchanged, so no event is re-emitted. The outbox sidesteps this by staging the publish atomically with the commit.

**Design note (hybrid, not relay-only):** an earlier draft of this plan said "the relay is the only publisher." That was wrong — it would make every alert wait for the next relay tick, regressing the headline latency. The shipped design publishes inline and uses the relay only as a backstop. (Credit: caught in a second-opinion review.) Guarantee is **at-least-once**: a crash between a successful publish and the mark yields at most one duplicate, which the alert path and the 48h freshness gate tolerate.

**Cheaper alternative (rejected):** re-publish on a "committed but unpublished" scan keyed off the `events` table — but firehose events (H1) have no `events` row, so a uniform outbox is cleaner and covers both.

---

## H1 — firehose marked "seen" before publish — FIXED

**Fixed:** `maybeFirehose` now runs `MarkFirehoseSeenTx` + `InsertOutbox` in **one transaction**, then publishes inline (same hybrid path as H2). "Recorded seen" and "will be published" commit together, so a publish failure no longer leaves a seen-but-never-alerted posting. Verified by `TestProcessorFirehose` (firehose outbox row published).

**Original mechanism (for the record):** `MarkFirehoseSeen` wrote the dedup row **before** the `events.firehose` publish. If the publish failed, the posting was already recorded as seen, so on the next poll `isNew` was false and it returned early without ever publishing. Dropped firehose alert — same outcome as H2, different spot. (A naive reorder — publish first, then mark seen — only swaps the failure mode to *double*-alerting, which is why the outbox is the right answer here too.)

---

## H3 — no delivery cap / backoff on consumers (independent) — FIXED

**Fixed:** `internal/bus/bus.go` — both consumers now set `MaxDeliver(5)`, `AckWait(30s)`, `BackOff([30s, 2m, 5m])`. Verified by `internal/bus/bus_integration_test.go` (asserts the created consumers carry the config). Deploying requires recreating the existing durables — see the operational step below; `make nats-reset` does this in dev.

**Where:** [internal/bus/bus.go:74-94](../../internal/bus/bus.go#L74) — both `SubscribeSignals` and `SubscribeEvents` use `ManualAck` + `AckExplicit` with **no `MaxDeliver`, `AckWait`, or `BackOff`**.

**Mechanism:** when a handler `Nak`s a message (e.g. a payload the normalizer can't parse), NATS re-delivers it immediately, forever. A persistent failure becomes a tight infinite loop: pegged CPU, log spam, and — because the bad message sits at the head — it **delays every alert behind it** (head-of-line blocking). Malformed JSON is already `Term`'d in the handlers, but *semantic* failures are not.

**Recommended fix (verified against nats.go v1.52.0):** add `nats.MaxDeliver(n)`, `nats.AckWait(d)`, and `nats.BackOff([...])` to both consumers. After `MaxDeliver` attempts NATS stops redelivering (the message is dropped from the work queue; an advisory is published). Backoff spaces out retries so a transient failure recovers without a hot loop.

**API notes / gotchas (verified against nats.go v1.52.0 source, not assumed):**
- `nats.BackOff` requires `MaxDeliver >= len(backoff)`, or consumer creation fails. Set `MaxDeliver` explicitly above the backoff length.
- **`js.Subscribe` binds to an existing durable; it does NOT update its config.** Verified: when the durable already exists, `js.Subscribe` takes the `case info != nil` branch and calls `processConsInfo` ([js.go:1559](https://github.com/nats-io/nats.go/blob/v1.52.0/js.go)), which **returns an error on mismatch** ("configuration requests max deliver to be N, but consumer's value is M"). So after this change ships, the processor/api services will **fail to start against the pre-existing "processor"/"dispatcher" consumers** until those are deleted and recreated with the new config. This is a *loud* failure (not a silent no-op), which is the safe outcome. **Operational step (required):** recreate the durable consumers on deploy. Dev NATS has no volume, so `make nats-reset` (force-recreate) wipes them; prod uses file storage and persists them, so delete them explicitly.
- **When `BackOff` is set, NATS uses `BackOff[0]` as the ack deadline and overrides the separate `AckWait`.** Found by the verification test (it reported `AckWait = 5s` when `BackOff[0]` was 5s, despite `AckWait(30s)` being passed). Consequence: `BackOff[0]` must exceed the handler's worst-case processing time. The dispatcher's Telegram client has a 10s timeout, so `BackOff[0] = 5s` would let a slow-but-successful send exceed the ack deadline and trigger a **duplicate alert**. Final values use `BackOff[0] = 30s` (and `ackWait` is kept equal to it for clarity).
- Terminal drop tradeoff: after `MaxDeliver` attempts the message is dropped. For the **SIGNALS** consumer this self-heals — the collector re-emits all active listings every poll (~5 min), so a dropped signal reappears. For the **EVENTS** consumer there is no re-emit, but events persist in Postgres and the EVENTS stream (90d) for manual replay, and a terminal drop is logged. Acceptable for single-user v1; `MaxDeliver` is set generously so only a genuinely-poison message is dropped.
- Optionally add `nats.MaxAckPending(n)` to bound in-flight redeliveries.

---

## Fix plan & sequencing — DONE

1. **H3** ✅ — consumer redelivery caps in `internal/bus/bus.go`; verified by `internal/bus/bus_integration_test.go`.
2. **H1 + H2** ✅ — one `event_outbox` table + hybrid (inline publish + relay backstop) fixes both. Migration `000007`, store methods in `internal/store/outbox.go`, staging in `processor.go`/`firehose.go`, relay goroutine in `cmd/processor`, tests in `internal/processor/integration_test.go`.

**Follow-ups (not blocking):**
- Add a Prometheus gauge on the unpublished-outbox backlog once metrics infra exists (DESIGN §7) — today the relay logs each non-empty sweep.
- Consider a periodic prune of old published `event_outbox` rows (they accumulate; a `published_at < now() - 7d` delete keeps the table small). Low urgency at this volume.
- Unify `MarkFirehoseSeen` / `MarkFirehoseSeenTx` once concurrent edits to `api.go` settle.

Each fix is recorded in [../changelog.md](../changelog.md) with its commit.
