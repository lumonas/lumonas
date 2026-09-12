package network

import "testing"

func TestConnectionValidationRejectsUnsafeValues(t *testing.T) {
	valid := Connection{ID: "lan", UUID: "12345678-1234", Name: "LAN", Interface: "en0", IPv4: IPConfig{Method: "auto"}, IPv6: IPConfig{Method: "disabled"}, MTU: 1500}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.Interface = "../../etc"
	if err := invalid.Validate(); err == nil {
		t.Fatal("path traversal interface name should fail")
	}
	invalid = valid
	invalid.IPv4 = IPConfig{Method: "manual", Addresses: []string{"192.168.1.10/24"}, Gateway: "not-an-ip"}
	if err := invalid.Validate(); err == nil {
		t.Fatal("invalid gateway should fail")
	}
}

func TestBindingsAndFirewallValidation(t *testing.T) {
	if err := ValidateBindings([]Binding{{Service: "ui", Port: 8080}, {Service: "ui", Port: 8081}}); err == nil {
		t.Fatal("duplicate service binding should fail")
	}
	policy := DefaultFirewallPolicy()
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	policy.Default = "drop"
	if err := policy.Validate(); err == nil {
		t.Fatal("unsupported firewall default should fail")
	}
	policy = DefaultFirewallPolicy()
	bindings := []Binding{{Service: "ftp", Port: 21, Enabled: true}}
	if err := ValidateExposure(bindings, policy); err == nil {
		t.Fatal("service without an allowed scope should fail")
	}
	policy.Services["ftp"] = FirewallService{LAN: true}
	rules, err := RenderNftables(policy, bindings)
	if err != nil || len(rules) == 0 {
		t.Fatalf("expected generated nftables rules: %q err=%v", rules, err)
	}
}
