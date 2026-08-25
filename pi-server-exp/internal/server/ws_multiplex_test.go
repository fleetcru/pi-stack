package server

import (
	"context"
	"sync"
	"testing"
)

func TestRouteMultiplexRejectsMissingRelayMessage(t *testing.T) {
	s := newTestServer(t, "")
	s.external.register("relay-1", t.TempDir(), "relay", "", "lease")
	out := make(chan any, 1)
	done := make(chan struct{})

	s.routeMultiplexCommand(context.Background(), "relay-1", RPCCommand{"type": "prompt"}, out, done)

	message := (<-out).(map[string]any)
	if message["type"] != "daemon_error" || message["error"] != "message is required" {
		t.Fatalf("unexpected response: %#v", message)
	}
}

func TestActivateMultiplexSubscriptionRejectsDuplicateAndClosedConnection(t *testing.T) {
	subs := map[string]*sessionSub{"reserved": {}}
	var mu sync.Mutex
	done := make(chan struct{})
	first := &sessionSub{unsub: func() {}}
	if !activateMultiplexSubscription("reserved", first, subs, &mu, done) {
		t.Fatal("reserved subscription was not activated")
	}
	if activateMultiplexSubscription("reserved", &sessionSub{}, subs, &mu, done) {
		t.Fatal("duplicate subscription was activated")
	}

	subs["closing"] = &sessionSub{}
	close(done)
	if activateMultiplexSubscription("closing", &sessionSub{}, subs, &mu, done) {
		t.Fatal("subscription activated after disconnect")
	}
	if _, ok := subs["closing"]; ok {
		t.Fatal("disconnect did not remove subscription reservation")
	}
}
