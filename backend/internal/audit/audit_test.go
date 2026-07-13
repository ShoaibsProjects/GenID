package audit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStore(t *testing.T) {
	s := NewStore(100)
	require.NotNil(t, s)
	assert.Equal(t, 100, s.cap)
	assert.Equal(t, 0, s.nextID)
}

func TestNewStore_DefaultCapacity(t *testing.T) {
	s := NewStore(0)
	assert.Equal(t, 10000, s.cap)
}

func TestAppendAndGet(t *testing.T) {
	s := NewStore(100)
	s.Append(Entry{Level: LevelInfo, Message: "test message", Service: "test"})

	e, ok := s.Get("LOG-1")
	require.True(t, ok)
	assert.Equal(t, LevelInfo, e.Level)
	assert.Equal(t, "test message", e.Message)
	assert.Equal(t, "test", e.Service)
	assert.Equal(t, "LOG-1", e.ID)
	assert.False(t, e.Timestamp.IsZero())
}

func TestAppendAutoTimestamp(t *testing.T) {
	s := NewStore(100)
	s.Append(Entry{Level: LevelInfo, Message: "msg"})

	e, ok := s.Get("LOG-1")
	require.True(t, ok)
	assert.WithinDuration(t, time.Now(), e.Timestamp, time.Second)
}

func TestAppendIncrementsID(t *testing.T) {
	s := NewStore(100)
	s.Append(Entry{Message: "first"})
	s.Append(Entry{Message: "second"})

	_, ok1 := s.Get("LOG-1")
	_, ok2 := s.Get("LOG-2")
	assert.True(t, ok1)
	assert.True(t, ok2)
}

func TestGetNotFound(t *testing.T) {
	s := NewStore(100)
	_, ok := s.Get("NONEXISTENT")
	assert.False(t, ok)
}

func TestListAll(t *testing.T) {
	s := NewStore(100)
	for i := 0; i < 5; i++ {
		s.Append(Entry{Level: LevelInfo, Message: "msg"})
	}

	entries := s.List(0, 0, "", "")
	assert.Len(t, entries, 5)
	// Reverse chronological order
	assert.Equal(t, "LOG-5", entries[0].ID)
	assert.Equal(t, "LOG-1", entries[4].ID)
}

func TestListWithLimit(t *testing.T) {
	s := NewStore(100)
	for i := 0; i < 10; i++ {
		s.Append(Entry{Level: LevelInfo})
	}

	entries := s.List(3, 0, "", "")
	assert.Len(t, entries, 3)
}

func TestListWithOffset(t *testing.T) {
	s := NewStore(100)
	for i := 0; i < 10; i++ {
		s.Append(Entry{Level: LevelInfo, Message: "msg"})
	}

	entries := s.List(5, 5, "", "")
	assert.Len(t, entries, 5)
	// Offset is applied before reversal, so offset=5 skips LOG1-5, leaving LOG6-10
	// Then reversed: LOG-10 is first
	assert.Equal(t, "LOG-10", entries[0].ID)
}

func TestListFilterByLevel(t *testing.T) {
	s := NewStore(100)
	s.Append(Entry{Level: LevelInfo, Message: "info"})
	s.Append(Entry{Level: LevelWarn, Message: "warn1"})
	s.Append(Entry{Level: LevelInfo, Message: "info2"})
	s.Append(Entry{Level: LevelError, Message: "error"})
	s.Append(Entry{Level: LevelWarn, Message: "warn2"})

	entries := s.List(0, 0, LevelWarn, "")
	assert.Len(t, entries, 2)
	for _, e := range entries {
		assert.Equal(t, LevelWarn, e.Level)
	}
}

func TestListFilterByPath(t *testing.T) {
	s := NewStore(100)
	s.Append(Entry{Level: LevelInfo, Path: "/api/v1/identities"})
	s.Append(Entry{Level: LevelInfo, Path: "/api/v1/connectors"})
	s.Append(Entry{Level: LevelInfo, Path: "/api/v1/identities/123"})

	entries := s.List(0, 0, "", "/api/v1/identities")
	assert.Len(t, entries, 2)
}

func TestListEmpty(t *testing.T) {
	s := NewStore(100)
	entries := s.List(10, 0, "", "")
	assert.Nil(t, entries)
}

func TestListOffsetExceeds(t *testing.T) {
	s := NewStore(100)
	s.Append(Entry{Level: LevelInfo})
	s.Append(Entry{Level: LevelInfo})

	entries := s.List(10, 10, "", "")
	assert.Nil(t, entries)
}

func TestCount(t *testing.T) {
	s := NewStore(100)
	s.Append(Entry{Level: LevelInfo})
	s.Append(Entry{Level: LevelWarn})
	s.Append(Entry{Level: LevelInfo})
	s.Append(Entry{Level: LevelError})

	assert.Equal(t, 4, s.Count(""))
	assert.Equal(t, 2, s.Count(LevelInfo))
	assert.Equal(t, 1, s.Count(LevelWarn))
	assert.Equal(t, 1, s.Count(LevelError))
	assert.Equal(t, 0, s.Count(LevelDebug))
}

func TestCapacityEviction(t *testing.T) {
	s := NewStore(3)
	for i := 0; i < 10; i++ {
		s.Append(Entry{Level: LevelInfo, Message: "msg"})
	}

	assert.Equal(t, 3, len(s.entries))
	_, ok := s.Get("LOG-1")
	assert.False(t, ok)
	_, ok = s.Get("LOG-10")
	assert.True(t, ok)
}

func TestStats(t *testing.T) {
	s := NewStore(10)
	s.Append(Entry{Level: LevelInfo})
	s.Append(Entry{Level: LevelWarn})
	s.Append(Entry{Level: LevelInfo})

	stats := s.Stats()
	assert.Equal(t, 3, stats.Total)
	assert.Equal(t, 10, stats.Capacity)
	assert.Equal(t, 30.0, stats.UsagePct)
	assert.Equal(t, 2, stats.ByLevel[LevelInfo])
	assert.Equal(t, 1, stats.ByLevel[LevelWarn])
}

func TestStatsEmpty(t *testing.T) {
	s := NewStore(100)
	stats := s.Stats()
	assert.Equal(t, 0, stats.Total)
	assert.Equal(t, 0.0, stats.UsagePct)
}

func TestConcurrentAppend(t *testing.T) {
	s := NewStore(1000)
	done := make(chan struct{})

	writer := func(n int) {
		for i := 0; i < n; i++ {
			s.Append(Entry{Level: LevelInfo, Message: "test"})
		}
		done <- struct{}{}
	}

	go writer(500)
	go writer(500)

	for i := 0; i < 2; i++ {
		<-done
	}

	assert.Equal(t, 1000, s.Count(""))
}

func TestListReverseOrder(t *testing.T) {
	s := NewStore(100)
	s.Append(Entry{Level: LevelInfo, Message: "first"})
	s.Append(Entry{Level: LevelWarn, Message: "second"})
	s.Append(Entry{Level: LevelError, Message: "third"})

	entries := s.List(0, 0, "", "")
	require.Len(t, entries, 3)
	assert.Equal(t, "third", entries[0].Message)
	assert.Equal(t, "second", entries[1].Message)
	assert.Equal(t, "first", entries[2].Message)
}
