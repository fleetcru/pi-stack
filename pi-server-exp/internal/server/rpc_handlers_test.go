package server

import "testing"

func TestExternalPromptDelivery(t *testing.T) {
	tests := []struct {
		action string
		want   string
	}{
		{action: "prompt", want: "prompt"},
		{action: "steer", want: "steer"},
		{action: "follow-up", want: "followUp"},
		{action: "follow_up", want: "followUp"},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			if got := externalPromptDelivery(tt.action); got != tt.want {
				t.Fatalf("externalPromptDelivery(%q) = %q, want %q", tt.action, got, tt.want)
			}
		})
	}
}
