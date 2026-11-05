package prompts

import "testing"

// The project prompt is a cache key input: it must not depend on Go's
// randomised map iteration order.
func TestProject_IsDeterministic(t *testing.T) {
	summaries := map[string]string{"c.pkg": "C", "a.pkg": "A", "b.pkg": "B", "d.pkg": "D", "e.pkg": "E"}
	_, first := Project("demo", summaries)
	for i := 0; i < 200; i++ {
		if _, again := Project("demo", summaries); again != first {
			t.Fatalf("prompt changed between calls:\n%s\n---\n%s", first, again)
		}
	}
}
