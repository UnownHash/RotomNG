package ws

import (
	"time"
)

// ConnStats tracks connection statistics including message counts and timestamps.
type ConnStats struct {
	ConnectedAt time.Time

	LastReceivedAt   time.Time
	MessagesReceived int64
	BytesReceived    int64

	LastSentAt   time.Time
	MessagesSent int64
	BytesSent    int64

	// LastPongAt is the time the most recent pong control frame was received. It
	// is tracked separately from LastReceivedAt (which counts data messages) so
	// that ping/pong keep-alive activity keeps LastSeenAt fresh without skewing
	// the message counts.
	LastPongAt time.Time
}

// Add merges another ConnStats into this one. Timestamps keep the latest of the
// two values; a zero time in other never moves a timestamp backward, since the
// zero time is before any real one.
func (st *ConnStats) Add(other ConnStats) {
	if other.ConnectedAt.After(st.ConnectedAt) {
		st.ConnectedAt = other.ConnectedAt
	}

	if other.LastReceivedAt.After(st.LastReceivedAt) {
		st.LastReceivedAt = other.LastReceivedAt
	}
	st.MessagesReceived += other.MessagesReceived
	st.BytesReceived += other.BytesReceived

	if other.LastSentAt.After(st.LastSentAt) {
		st.LastSentAt = other.LastSentAt
	}
	st.MessagesSent += other.MessagesSent
	st.BytesSent += other.BytesSent

	if other.LastPongAt.After(st.LastPongAt) {
		st.LastPongAt = other.LastPongAt
	}
}

// LastSeenAt returns the most recent activity timestamp for this connection,
// including ping/pong keep-alive activity.
func (st *ConnStats) LastSeenAt() time.Time {
	latest := st.ConnectedAt
	for _, t := range []time.Time{st.LastReceivedAt, st.LastSentAt, st.LastPongAt} {
		if t.After(latest) {
			latest = t
		}
	}
	return latest
}

func (st *ConnStats) setMessageReceived(now time.Time, n int64) {
	st.LastReceivedAt = now
	st.MessagesReceived++
	st.BytesReceived += n
}

func (st *ConnStats) setMessageSent(now time.Time, n int64) {
	st.LastSentAt = now
	st.MessagesSent++
	st.BytesSent += n
}

func (st *ConnStats) setPongReceived(now time.Time) {
	st.LastPongAt = now
}
