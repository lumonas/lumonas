package quotas

import "testing"

func TestPolicyValidation(t *testing.T) {
	valid := Policy{ID: "q1", TargetType: "share", TargetID: "share-1", LimitBytes: 1 << 30, WarningPercent: 85}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, value := range []Policy{{ID: "q", TargetType: "disk", TargetID: "x", LimitBytes: 1 << 30, WarningPercent: 85}, {ID: "q", TargetType: "user", TargetID: "x", LimitBytes: 100, WarningPercent: 85}, {ID: "q", TargetType: "group", TargetID: "x", LimitBytes: 1 << 30, WarningPercent: 100}} {
		if err := value.Validate(); err == nil {
			t.Fatalf("invalid policy accepted: %#v", value)
		}
	}
}
