package logevent

import (
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSingleSubscriberReceivesPublishedEvent(t *testing.T) {
	hub := NewHub()
	ch, unsubscribe := hub.Subscribe("task-1")
	defer unsubscribe()

	event := testEvent("task-1", 1)
	hub.Publish(event)

	got := receiveEvent(t, ch)
	if !reflect.DeepEqual(got, event) {
		t.Fatalf("received event = %#v, want %#v", got, event)
	}
}

func TestMultipleSubscribersReceiveSameEvent(t *testing.T) {
	hub := NewHub()
	ch1, unsubscribe1 := hub.Subscribe("task-1")
	defer unsubscribe1()
	ch2, unsubscribe2 := hub.Subscribe("task-1")
	defer unsubscribe2()
	ch3, unsubscribe3 := hub.Subscribe("task-1")
	defer unsubscribe3()

	event := testEvent("task-1", 1)
	hub.Publish(event)

	for i, ch := range []<-chan Event{ch1, ch2, ch3} {
		if got := receiveEvent(t, ch); !reflect.DeepEqual(got, event) {
			t.Fatalf("subscriber %d received event = %#v, want %#v", i+1, got, event)
		}
	}
}

func TestTaskSubscribersAreIsolated(t *testing.T) {
	hub := NewHub()
	taskA, unsubscribeA := hub.Subscribe("task-a")
	defer unsubscribeA()
	taskB, unsubscribeB := hub.Subscribe("task-b")
	defer unsubscribeB()

	eventA := testEvent("task-a", 1)
	hub.Publish(eventA)

	if got := receiveEvent(t, taskA); !reflect.DeepEqual(got, eventA) {
		t.Fatalf("task-a subscriber received event = %#v, want %#v", got, eventA)
	}
	assertNoEvent(t, taskB)
}

func TestSlowSubscriberDropsEventsWithoutBlockingOthers(t *testing.T) {
	hub := NewHub()
	slow, unsubscribeSlow := hub.Subscribe("task-1")
	defer unsubscribeSlow()

	for i := 0; i < subscriberBufferSize; i++ {
		hub.Publish(testEvent("task-1", i))
	}
	if got := len(slow); got != subscriberBufferSize {
		t.Fatalf("slow subscriber buffered events = %d, want %d", got, subscriberBufferSize)
	}

	fast, unsubscribeFast := hub.Subscribe("task-1")
	defer unsubscribeFast()
	event := testEvent("task-1", 99)

	done := make(chan struct{})
	go func() {
		defer close(done)
		hub.Publish(event)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Publish blocked on a full subscriber channel")
	}

	if got := receiveEvent(t, fast); !reflect.DeepEqual(got, event) {
		t.Fatalf("fast subscriber received event = %#v, want %#v", got, event)
	}
}

func TestUnsubscribeIsIdempotentAndStopsNewEvents(t *testing.T) {
	hub := NewHub()
	ch, unsubscribe := hub.Subscribe("task-1")

	unsubscribe()
	unsubscribe()
	hub.Publish(testEvent("task-1", 1))

	select {
	case event, ok := <-ch:
		if ok {
			t.Fatalf("received event after unsubscribe: %#v", event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("subscription channel was not closed after unsubscribe")
	}
}

func TestPublishWithNoSubscribersDoesNotBlock(t *testing.T) {
	hub := NewHub()

	done := make(chan struct{})
	go func() {
		defer close(done)
		hub.Publish(testEvent("task-1", 1))
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Publish blocked with no subscribers")
	}
}

func TestConcurrentSubscribePublishUnsubscribeIsSafe(t *testing.T) {
	hub := NewHub()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			taskID := "task-" + string(rune('a'+id%5))

			for seq := 0; seq < 100; seq++ {
				ch, unsubscribe := hub.Subscribe(taskID)
				hub.Publish(testEvent(taskID, seq))
				select {
				case <-ch:
				default:
				}
				unsubscribe()
				unsubscribe()
			}
		}(i)
	}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			taskID := "task-" + string(rune('a'+id%5))

			for seq := 0; seq < 500; seq++ {
				hub.Publish(testEvent(taskID, seq))
			}
		}(i)
	}

	wg.Wait()
}

func TestEventDoesNotContainLargeOrSensitiveFields(t *testing.T) {
	eventType := reflect.TypeOf(Event{})
	for i := 0; i < eventType.NumField(); i++ {
		field := eventType.Field(i)
		name := strings.ToLower(field.Name)
		jsonTag := strings.ToLower(field.Tag.Get("json"))

		for _, forbidden := range []string{"request", "response", "body", "header"} {
			if strings.Contains(name, forbidden) || strings.Contains(jsonTag, forbidden) {
				t.Fatalf("Event field %q must not contain %q", field.Name, forbidden)
			}
		}
	}
}

func testEvent(taskID string, seqNo int) Event {
	return Event{
		TaskID:    taskID,
		AccountID: "account-1",
		SeqNo:     seqNo,
		StepName:  "login",
		Success:   true,
		CostMs:    123,
		Timestamp: time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC),
	}
}

func receiveEvent(t *testing.T, ch <-chan Event) Event {
	t.Helper()

	select {
	case event, ok := <-ch:
		if !ok {
			t.Fatal("subscription channel closed before receiving event")
		}
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
		return Event{}
	}
}

func assertNoEvent(t *testing.T, ch <-chan Event) {
	t.Helper()

	select {
	case event, ok := <-ch:
		if ok {
			t.Fatalf("unexpected event received: %#v", event)
		}
		t.Fatal("subscription channel closed unexpectedly")
	case <-time.After(50 * time.Millisecond):
	}
}
