package ws

import (
	"testing"
	"time"
)

func TestConnStats_setMessageReceived(t *testing.T) {
	var st ConnStats
	now := time.Now()
	st.setMessageReceived(now, 100)
	st.setMessageReceived(now.Add(time.Second), 200)

	if st.MessagesReceived != 2 {
		t.Errorf("expected 2 messages received, got %d", st.MessagesReceived)
	}
	if st.BytesReceived != 300 {
		t.Errorf("expected 300 bytes received, got %d", st.BytesReceived)
	}
	if !st.LastReceivedAt.Equal(now.Add(time.Second)) {
		t.Error("LastReceivedAt should be the most recent time")
	}
}

func TestConnStats_setMessageSent(t *testing.T) {
	var st ConnStats
	now := time.Now()
	st.setMessageSent(now, 50)
	st.setMessageSent(now.Add(time.Second), 75)

	if st.MessagesSent != 2 {
		t.Errorf("expected 2 messages sent, got %d", st.MessagesSent)
	}
	if st.BytesSent != 125 {
		t.Errorf("expected 125 bytes sent, got %d", st.BytesSent)
	}
	if !st.LastSentAt.Equal(now.Add(time.Second)) {
		t.Error("LastSentAt should be the most recent time")
	}
}

func TestConnStats_Add(t *testing.T) {
	t1 := time.Now().Add(-time.Hour)
	t2 := time.Now()

	st1 := ConnStats{
		ConnectedAt:      t1,
		LastReceivedAt:   t1,
		MessagesReceived: 5,
		BytesReceived:    500,
		LastSentAt:       t1,
		MessagesSent:     3,
		BytesSent:        300,
	}

	st2 := ConnStats{
		ConnectedAt:      t2,
		LastReceivedAt:   t2,
		MessagesReceived: 2,
		BytesReceived:    200,
		LastSentAt:       t2,
		MessagesSent:     1,
		BytesSent:        100,
	}

	st1.Add(st2)

	if !st1.ConnectedAt.Equal(t2) {
		t.Error("ConnectedAt should be updated to other's value")
	}
	if !st1.LastReceivedAt.Equal(t2) {
		t.Error("LastReceivedAt should be updated to other's value")
	}
	if st1.MessagesReceived != 7 {
		t.Errorf("expected 7 messages received, got %d", st1.MessagesReceived)
	}
	if st1.BytesReceived != 700 {
		t.Errorf("expected 700 bytes received, got %d", st1.BytesReceived)
	}
	if !st1.LastSentAt.Equal(t2) {
		t.Error("LastSentAt should be updated to other's value")
	}
	if st1.MessagesSent != 4 {
		t.Errorf("expected 4 messages sent, got %d", st1.MessagesSent)
	}
	if st1.BytesSent != 400 {
		t.Errorf("expected 400 bytes sent, got %d", st1.BytesSent)
	}
}

func TestConnStats_Add_ZeroFields(t *testing.T) {
	t1 := time.Now()
	st := ConnStats{
		ConnectedAt:    t1,
		LastReceivedAt: t1,
		LastSentAt:     t1,
	}

	// Adding zero-value stats should not overwrite times
	st.Add(ConnStats{})

	if !st.ConnectedAt.Equal(t1) {
		t.Error("ConnectedAt should not be changed by zero value")
	}
	if !st.LastReceivedAt.Equal(t1) {
		t.Error("LastReceivedAt should not be changed by zero value")
	}
	if !st.LastSentAt.Equal(t1) {
		t.Error("LastSentAt should not be changed by zero value")
	}
}

func TestConnStats_LastSeenAt(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	// Only connected, no messages
	st := &ConnStats{ConnectedAt: now}
	if !st.LastSeenAt().Equal(now) {
		t.Error("LastSeenAt should return ConnectedAt when no messages")
	}

	// Received is most recent
	st = &ConnStats{ConnectedAt: past, LastReceivedAt: future, LastSentAt: now}
	if !st.LastSeenAt().Equal(future) {
		t.Error("LastSeenAt should return LastReceivedAt when it's most recent")
	}

	// Sent is most recent
	st = &ConnStats{ConnectedAt: past, LastReceivedAt: now, LastSentAt: future}
	if !st.LastSeenAt().Equal(future) {
		t.Error("LastSeenAt should return LastSentAt when it's most recent")
	}

	// Connected is most recent
	st = &ConnStats{ConnectedAt: future, LastReceivedAt: now, LastSentAt: past}
	if !st.LastSeenAt().Equal(future) {
		t.Error("LastSeenAt should return ConnectedAt when it's most recent")
	}

	// Pong is most recent
	st = &ConnStats{ConnectedAt: past, LastReceivedAt: now, LastSentAt: now, LastPongAt: future}
	if !st.LastSeenAt().Equal(future) {
		t.Error("LastSeenAt should return LastPongAt when it's most recent")
	}
}

func TestConnStats_setPongReceived(t *testing.T) {
	var st ConnStats
	now := time.Now()
	st.setPongReceived(now)

	if !st.LastPongAt.Equal(now) {
		t.Error("LastPongAt should be set to the pong time")
	}
	// A pong must not be counted as a data message.
	if st.MessagesReceived != 0 || st.BytesReceived != 0 {
		t.Errorf("pong should not affect message counts, got messages=%d bytes=%d", st.MessagesReceived, st.BytesReceived)
	}
	if !st.LastSeenAt().Equal(now) {
		t.Error("LastSeenAt should reflect the pong time")
	}
}

func TestConnStats_Add_LastPongAt(t *testing.T) {
	t1 := time.Now().Add(-time.Hour)
	t2 := time.Now()

	st := ConnStats{LastPongAt: t1}
	st.Add(ConnStats{LastPongAt: t2})
	if !st.LastPongAt.Equal(t2) {
		t.Error("LastPongAt should be updated to other's later value")
	}

	// A zero LastPongAt must not clobber the existing value.
	st.Add(ConnStats{})
	if !st.LastPongAt.Equal(t2) {
		t.Error("LastPongAt should not be changed by zero value")
	}

	// An earlier LastPongAt must not move the timestamp backward.
	st.Add(ConnStats{LastPongAt: t1})
	if !st.LastPongAt.Equal(t2) {
		t.Error("LastPongAt should keep the latest value, not an earlier one")
	}
}
