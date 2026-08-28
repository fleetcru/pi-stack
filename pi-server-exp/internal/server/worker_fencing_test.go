package server

import "testing"

func TestWorkerGenerationChangesOnUpdate(t *testing.T) {
	registry := NewWorkerRegistry("")
	worker := Worker{ID: "worker-1", URL: "https://example.com"}
	if err := registry.Add(worker); err != nil {
		t.Fatal(err)
	}
	first, ok := registry.Get(worker.ID)
	if !ok {
		t.Fatal("worker was not added")
	}
	updated := first
	updated.Tags = []string{"updated"}
	if err := registry.Update(updated); err != nil {
		t.Fatal(err)
	}
	second, _ := registry.Get(worker.ID)
	if second.Generation <= first.Generation {
		t.Fatalf("worker generation did not advance: %d -> %d", first.Generation, second.Generation)
	}
	if registry.CurrentGeneration(worker.ID) != second.Generation {
		t.Fatal("current generation disagrees with worker snapshot")
	}
}
