package targets

import (
	"encoding/json"
	"testing"
)

func TestTurnEndProjectsToStopAndRemovesCleanly(t *testing.T) {
	original := `{"permissions":{"allow":["Read"]},"hooks":{"Stop":[{"hooks":[{"type":"command","command":"my-stop-hook"}]}]}}`
	if !(claudeCode{}).Supports("turn-end") {
		t.Fatal("Claude must deliver turn-end")
	}
	merged, err := mergeClaudeSettings(original, []string{"turn-end"}, true)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(merged), &got); err != nil {
		t.Fatal(err)
	}
	stops := got["hooks"].(map[string]any)["Stop"].([]any)
	if len(stops) != 2 {
		t.Fatalf("want user's hook plus ilk's hook: %s", merged)
	}
	group := stops[1].(map[string]any)
	if _, ok := group["matcher"]; ok {
		t.Fatal("Stop does not use a matcher")
	}
	hook := group["hooks"].([]any)[0].(map[string]any)
	if hook["command"] != "ilk hook run turn-end" {
		t.Fatalf("wrong command: %v", hook)
	}
	again, err := mergeClaudeSettings(merged, []string{"turn-end"}, true)
	if err != nil || again != merged {
		t.Fatalf("not idempotent: %v", err)
	}
	removed, err := mergeClaudeSettings(merged, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	var want any
	if err := json.Unmarshal([]byte(original), &want); err != nil {
		t.Fatal(err)
	}
	var actual any
	if err := json.Unmarshal([]byte(removed), &actual); err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(actual)
	b, _ := json.Marshal(want)
	if string(a) != string(b) {
		t.Fatalf("removal changed user settings: %s", removed)
	}
}
