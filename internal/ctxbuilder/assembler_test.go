package ctxbuilder

import (
	"strings"
	"testing"
)

func TestAssembleSingleShot(t *testing.T) {
	got := Assemble(Context{System: "sys", Rules: []string{"r1"}, Skills: []string{"s1"}, Task: "do x"})
	for _, w := range []string{"sys", "[RULES]", "r1", "[SKILLS]", "s1", "[TASK]", "do x"} {
		if !strings.Contains(got, w) {
			t.Errorf("assembled prompt missing %q:\n%s", w, got)
		}
	}
	if strings.Contains(got, "CONVERSATION") {
		t.Errorf("no history should mean no conversation section:\n%s", got)
	}
}

func TestAssembleWithHistory(t *testing.T) {
	got := Assemble(Context{
		Task: "now",
		History: []Exchange{
			{Role: "User", Text: "hi"},
			{Role: "Assistant (claude)", Text: "hello"},
		},
	})
	for _, w := range []string{"[CONVERSATION SO FAR]", "User: hi", "Assistant (claude): hello", "[TASK]", "now"} {
		if !strings.Contains(got, w) {
			t.Errorf("assembled chat prompt missing %q:\n%s", w, got)
		}
	}
	// History must come before the current task.
	if strings.Index(got, "CONVERSATION SO FAR") > strings.Index(got, "[TASK]") {
		t.Errorf("history should precede the task:\n%s", got)
	}
}
