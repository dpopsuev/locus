package store_test

import (
	"encoding/json"
	"testing"

	"github.com/dpopsuev/locus/internal/store"
)

func TestDesiredState_RolesField(t *testing.T) {
	ds := store.DesiredState{
		Layers: []string{"domain", "app", "infra"},
		Roles: map[string]string{
			"internal/driver": "port",
			"internal/http":   "adapter",
		},
	}

	data, err := json.Marshal(ds)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got store.DesiredState
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(got.Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(got.Roles))
	}
	if got.Roles["internal/driver"] != "port" {
		t.Errorf("expected role 'port' for internal/driver, got %q", got.Roles["internal/driver"])
	}
	if got.Roles["internal/http"] != "adapter" {
		t.Errorf("expected role 'adapter' for internal/http, got %q", got.Roles["internal/http"])
	}
}

func TestDesiredState_AcceptedField(t *testing.T) {
	ds := store.DesiredState{
		Layers: []string{"domain", "app"},
		Accepted: []store.AcceptedViolation{
			{
				Component: "internal/staff",
				Principle: "SRP",
				Reason:    "intentional facade — planned split tracked in TSK-312",
			},
			{
				Component: "internal/driver",
				Principle: "DIP",
				Reason:    "false positive — driver is a port, not an adapter",
			},
		},
	}

	data, err := json.Marshal(ds)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got store.DesiredState
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(got.Accepted) != 2 {
		t.Fatalf("expected 2 accepted violations, got %d", len(got.Accepted))
	}

	a := got.Accepted[0]
	if a.Component != "internal/staff" {
		t.Errorf("expected component 'internal/staff', got %q", a.Component)
	}
	if a.Principle != "SRP" {
		t.Errorf("expected principle 'SRP', got %q", a.Principle)
	}
	if a.Reason != "intentional facade — planned split tracked in TSK-312" {
		t.Errorf("unexpected reason: %q", a.Reason)
	}

	b := got.Accepted[1]
	if b.Component != "internal/driver" {
		t.Errorf("expected component 'internal/driver', got %q", b.Component)
	}
	if b.Principle != "DIP" {
		t.Errorf("expected principle 'DIP', got %q", b.Principle)
	}
}
