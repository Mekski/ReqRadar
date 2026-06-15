# Alert-loss trio (H1 / H2 / H3)

**Status:** open (as of 2026-06-15)
**Source:** [2026-06-15 progress audit](../audits/2026-06-15-progress-audit.md)
**Theme:** three ways a genuinely-new posting's alert can be **silently dropped or stalled**. None corrupt data — Postgres stays correct — they are *alert-delivery* reliability gaps. They matter because the headline claim is "sub-minute detection-to-alert," and they are exactly what an interviewer probes when you say "event-driven, at-least-once delivery."

The pipeline:

```
collector → NATS(SIGNALS) → processor → [1. write Postgres  2. publish events.* to NATS] → dispatcher → Telegram
```

Findings H1 and H2 are two faces of the same **dual-write problem**: the processor writes to Postgres *and* publishes to NATS, and those two systems can't be updated atomically. H3 is independent (consumer-side resilience).

> Severity in practice today: **low** — single user, NATS healthy on localhost, so none fire in normal operation. They become real on a deployed VM where NATS can hiccup, and they are the robustness story behind the resume claim.

---

## H2 — publish-after-commit (primary)

**Where:** [internal/processor/processor.go:169-175](../../internal/processor/processor.go#L169) (commit then publish) and [internal/processor/processor.go:193-202](../../internal/processor/processor.go#L193) (`publish` logs on failure, does not propagate).

**Mechanism:** the DB transaction commits the `postings` + `events` rows first; then `p.publish` sends to NATS and, on failure, only logs (`// publish only delays the alert. Logged, not fatal.`). If NATS is briefly unavailable at that moment, the event is permanently in Postgres but was **never enqueued**, so the dispatcher never sees it → no alert. On the next poll the same posting re-arrives, its content hash matches the stored row, and processing takes the `default:` "unchanged" branch ([processor.go:159](../../internal/processor/processor.go#L159)) — so it **never re-emits**. The alert is lost for good, and nothing looks broken.

**Why the obvious fix doesn't work:** "return an error so NATS re-delivers the signal" fails because the transaction already committed — on redelivery the posting exists and is unchanged, so no event is re-emitted.

**Recommended fix — transactional outbox:** inside the same transaction that writes the posting/event, also insert the to-be-published message into an `event_outbox` table. A relay (a goroutine in the processor) polls unpublished outbox rows, publishes them to NATS, and marks them published. Now the publish cannot be lost: a failed publish leaves the row un-marked and it is retried. This is the textbook fix for dual-write inconsistency and a strong interview answer. Replay stays idempotent (the relay is the only publisher).

**Cheaper alternative (rejected):** re-publish on a "committed but unpublished" scan keyed off the `events` table — but firehose events (H1) have no `events` row, so a uniform outbox is cleaner and covers both.

---

## H1 — firehose marked "seen" before publish

**Where:** [internal/processor/firehose.go:29-40](../../internal/processor/firehose.go#L29).

**Mechanism:** `MarkFirehoseSeen` writes the dedup row **before** the `events.firehose` is published. If the publish then fails, the posting is already recorded as seen, so on the next poll `isNew` is false ([firehose.go:33](../../internal/processor/firehose.go#L33)) and it returns early without ever publishing. Dropped firehose alert — same outcome as H2, different spot.

**Recommended fix:** route firehose events through the **same outbox** as H2 (insert the firehose event into `event_outbox` in the same tx as the `firehose_seen` write). This makes "recorded seen" and "will be published" atomic. A naive reorder (publish first, then mark seen) only swaps the failure mode to *double*-alerting, so the outbox is the clean answer here too.

---

## H3 — no delivery cap / backoff on consumers (independent)

**Where:** [internal/bus/bus.go:74-94](../../internal/bus/bus.go#L74) — both `SubscribeSignals` and `SubscribeEvents` use `ManualAck` + `AckExplicit` with **no `MaxDeliver`, `AckWait`, or `BackOff`**.

**Mechanism:** when a handler `Nak`s a message (e.g. a payload the normalizer can't parse), NATS re-delivers it immediately, forever. A persistent failure becomes a tight infinite loop: pegged CPU, log spam, and — because the bad message sits at the head — it **delays every alert behind it** (head-of-line blocking). Malformed JSON is already `Term`'d in the handlers, but *semantic* failures are not.

**Recommended fix (verified against nats.go v1.52.0):** add `nats.MaxDeliver(n)`, `nats.AckWait(d)`, and `nats.BackOff([...])` to both consumers. After `MaxDeliver` attempts NATS stops redelivering (the message is dropped from the work queue; an advisory is published). Backoff spaces out retries so a transient failure recovers without a hot loop.

**API notes / gotchas (researched, not assumed):**
- `nats.BackOff` requires `MaxDeliver >= len(backoff)`, or consumer creation fails. Set `MaxDeliver` explicitly above the backoff length.
- Changing options on an **existing durable consumer** ("processor"/"dispatcher") triggers a consumer *update*; recent NATS servers allow updating `AckWait`/`MaxDeliver`/`BackOff`/`MaxAckPending` on a push consumer, but if an update is rejected the service fails to start. **Operational step:** after deploying this change, the existing durable consumers may need to be deleted so they're recreated with the new config. (Dev NATS has no volume, so `docker compose up -d --force-recreate nats` wipes them; prod uses file storage and persists them.)
- Optionally add `nats.MaxAckPending(n)` to bound in-flight redeliveries.

---

## Fix plan & sequencing

1. **H3** first — independent, low-risk, and directly testable (a poison message must be delivered exactly `MaxDeliver` times then stop). No schema change.
2. **H1 + H2 together** via one `event_outbox` table + relay — they are the same dual-write problem; one mechanism fixes both. Requires a forward-only migration, store methods, a relay goroutine in `cmd/processor`, and tests (publish failure does not drop; replay stays idempotent).

Each fix updates this doc's status and is recorded in [../changelog.md](../changelog.md) with its commit.
