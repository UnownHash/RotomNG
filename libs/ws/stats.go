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
}

// Add merges another ConnStats into this one.
func (st *ConnStats) Add(other ConnStats) {
	if !other.ConnectedAt.IsZero() {
		st.ConnectedAt = other.ConnectedAt
	}

	if !other.LastReceivedAt.IsZero() {
		st.LastReceivedAt = other.LastReceivedAt
	}
	st.MessagesReceived += other.MessagesReceived
	st.BytesReceived += other.BytesReceived

	if !other.LastSentAt.IsZero() {
		st.LastSentAt = other.LastSentAt
	}
	st.MessagesSent += other.MessagesSent
	st.BytesSent += other.BytesSent
}

// LastSeenAt returns the most recent activity timestamp for this connection.
func (st *ConnStats) LastSeenAt() time.Time {
	sentRecvMax := func() time.Time {
		if st.LastReceivedAt.After(st.LastSentAt) {
			return st.LastReceivedAt
		}
		return st.LastSentAt
	}()
	if sentRecvMax.After(st.ConnectedAt) {
		return sentRecvMax
	}
	return st.ConnectedAt
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
