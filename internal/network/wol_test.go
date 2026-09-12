package network

import (
	"context"
	"reflect"
	"testing"
)

func TestParseWOL(t *testing.T) {
	supported, enabled := parseWOL("Supports Wake-on: pumbg\nWake-on: g\n")
	if !supported || !enabled {
		t.Fatalf("expected enabled WOL capability, got supported=%v enabled=%v", supported, enabled)
	}
	supported, enabled = parseWOL("Supports Wake-on: pumbg\nWake-on: d\n")
	if !supported || enabled {
		t.Fatalf("expected disabled WOL capability, got supported=%v enabled=%v", supported, enabled)
	}
}

func TestSetWOLUsesExactAllowListedArguments(t *testing.T) {
	var calls [][]string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return nil, nil
	}
	if err := SetWOL(context.Background(), "enp1s0", true, run); err != nil {
		t.Fatal(err)
	}
	if err := SetWOL(context.Background(), "enp1s0", false, run); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"ethtool", "-s", "enp1s0", "wol", "g"}, {"ethtool", "-s", "enp1s0", "wol", "d"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected ethtool calls: %#v", calls)
	}
}

func TestSetWOLRejectsShellSyntaxInInterface(t *testing.T) {
	called := false
	err := SetWOL(context.Background(), "enp1s0; touch /tmp/pwned", true, func(context.Context, string, ...string) ([]byte, error) {
		called = true
		return nil, nil
	})
	if err == nil || called {
		t.Fatalf("unsafe interface was accepted: err=%v called=%v", err, called)
	}
}
