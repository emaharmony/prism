package v2

import (
	"testing"
	"time"
)

func TestTimeoutForDefaultsWhenNoOverride(t *testing.T) {
	dm := NewDelegationManager("s", "s.complete")
	if got := dm.timeoutFor("anyagent"); got != defaultDelegationTimeout {
		t.Fatalf("unknown agent should use default %v, got %v", defaultDelegationTimeout, got)
	}
}

func TestSetAgentAndDefaultTimeout(t *testing.T) {
	dm := NewDelegationManager("s", "s.complete")
	dm.SetAgentTimeout("coder", 5*time.Minute)
	dm.SetDefaultTimeout(45 * time.Minute)
	if got := dm.timeoutFor("coder"); got != 5*time.Minute {
		t.Fatalf("per-agent override wrong: %v", got)
	}
	if got := dm.timeoutFor("other"); got != 45*time.Minute {
		t.Fatalf("default override wrong: %v", got)
	}
	// no-ops
	dm.SetAgentTimeout("", time.Minute)
	dm.SetAgentTimeout("x", 0)
	dm.SetDefaultTimeout(0)
	if got := dm.timeoutFor("other"); got != 45*time.Minute {
		t.Fatalf("no-op setters changed state: %v", got)
	}
}

func TestApplyTimeoutConfig(t *testing.T) {
	dm := NewDelegationManager("s", "s.complete")
	dm.ApplyTimeoutConfig(map[string]string{
		"reviewer": "10m",
		"default":  "30m",
		"bad":      "not-a-duration",
		"zero":     "0s",
	})
	if got := dm.timeoutFor("reviewer"); got != 10*time.Minute {
		t.Fatalf("reviewer timeout wrong: %v", got)
	}
	if got := dm.timeoutFor("anyone"); got != 30*time.Minute {
		t.Fatalf("default from config wrong: %v", got)
	}
	// invalid / zero entries are skipped (fall back to the configured default)
	if got := dm.timeoutFor("bad"); got != 30*time.Minute {
		t.Fatalf("invalid duration should be skipped: %v", got)
	}
	if got := dm.timeoutFor("zero"); got != 30*time.Minute {
		t.Fatalf("zero duration should be skipped: %v", got)
	}
}

// The deadline in a built packet reflects the configured timeout.
func TestBuildTaskPacketUsesConfiguredTimeout(t *testing.T) {
	dm := NewDelegationManager("s", "s.complete")
	dm.SetAgentTimeout("coder", time.Hour)
	pkt := dm.BuildTaskPacket(PlanTask{ID: "T1", Agent: "coder"})
	deadline, err := time.Parse(time.RFC3339, pkt.Deadline)
	if err != nil {
		t.Fatalf("bad deadline: %v", err)
	}
	// ~1 hour out (allow generous slack for slow CI).
	if d := time.Until(deadline); d < 50*time.Minute || d > 70*time.Minute {
		t.Fatalf("deadline not ~1h: %v", d)
	}
}
