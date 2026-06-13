// Package signal defines the universal envelope every collector emits and the
// processor consumes. It is published to NATS subject signals.raw.<source>.
// See DESIGN.md §3.1.
package signal

import (
	"encoding/json"
	"time"
)

// Kind classifies what a RawSignal represents.
type Kind string

const (
	KindPosting    Kind = "posting"
	KindBlogPost   Kind = "blog_post"
	KindDiscussion Kind = "discussion"
)

// RawSignal is the source-agnostic envelope. Collectors fetch, stamp, hash, and
// emit; all interpretation happens downstream in the processor so a parser fix
// can be replayed over stored raw signals rather than re-fetched.
//
// EventTime and ObservedAt are deliberately distinct: EventTime is when the
// thing happened (during backfill this may be years in the past), ObservedAt is
// when we saw it (t0 for the detection-to-alert latency measurement). Never
// conflate them — timing analytics group by EventTime.
type RawSignal struct {
	Source      string          `json:"source"`
	ExternalID  string          `json:"external_id"`
	Kind        Kind            `json:"kind"`
	EventTime   time.Time       `json:"event_time"`
	ObservedAt  time.Time       `json:"observed_at"`
	Payload     json.RawMessage `json:"payload"`
	ContentHash string          `json:"content_hash"`
}
