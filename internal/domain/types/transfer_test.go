package types

import (
	"encoding/json"
	"testing"
)

func TestDigestValidate(t *testing.T) {
	tests := []struct {
		name    string
		digest  Digest
		wantErr bool
	}{
		// Valid cases
		{name: "valid sha256 digest", digest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", wantErr: false},
		{name: "empty is valid (optional)", digest: "", wantErr: false},
		{name: "leading/trailing spaces are allowed", digest: " sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef ", wantErr: false},

		// Invalid cases
		{name: "whitespace-only is invalid", digest: "   ", wantErr: true},
		{name: "wrong prefix", digest: "sha512:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", wantErr: true},
		{name: "uppercase hex rejected", digest: "sha256:0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef", wantErr: true},
		{name: "too short hex", digest: "sha256:0123456789abcdef", wantErr: true},
		{name: "too long hex", digest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef00", wantErr: true},
		{name: "missing colon", digest: "sha2560123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", wantErr: true},
		{name: "invalid chars in hex", digest: "sha256:0123456789ghijkl0123456789abcdef0123456789abcdef0123456789abcdef", wantErr: true},
		{name: "bare hex without prefix", digest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", wantErr: true},
		{name: "random string", digest: "not-a-digest", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.digest.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Digest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDigestJSON(t *testing.T) {
	valid := Digest("sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	data, err := json.Marshal(valid)
	if err != nil {
		t.Fatalf("Marshal valid digest: %v", err)
	}
	var decoded Digest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal valid digest: %v", err)
	}
	if decoded != valid {
		t.Errorf("round-trip mismatch: got %q, want %q", decoded, valid)
	}

	// Invalid digest should fail to unmarshal.
	if err := json.Unmarshal([]byte(`"invalid-digest"`), &decoded); err == nil {
		t.Error("expected error unmarshaling invalid digest")
	}
	if err := json.Unmarshal([]byte(`"   "`), &decoded); err == nil {
		t.Error("expected error unmarshaling whitespace-only digest")
	}
}

func TestSlotIDValidate(t *testing.T) {
	tests := []struct {
		name    string
		slotID  SlotID
		wantErr bool
	}{
		{name: "valid slot id", slotID: "slot-123", wantErr: false},
		{name: "valid alphanumeric", slotID: "abc123XYZ", wantErr: false},
		{name: "valid with dashes", slotID: "slot-abc-123", wantErr: false},
		{name: "empty is invalid", slotID: "", wantErr: true},
		{name: "whitespace-only is invalid", slotID: "   ", wantErr: true},
		{name: "leading/trailing spaces is invalid", slotID: " slot-123 ", wantErr: true},
		{name: "contains slash is invalid", slotID: "slot/123", wantErr: true},
		{name: "contains question mark is invalid", slotID: "slot?123", wantErr: true},
		{name: "contains space is invalid", slotID: "slot 123", wantErr: true},
		{name: "contains tab is invalid", slotID: "slot\t123", wantErr: true},
		{name: "contains newline is invalid", slotID: "slot\n123", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.slotID.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("SlotID.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSlotIDJSON(t *testing.T) {
	valid := SlotID("slot-123")
	data, err := json.Marshal(valid)
	if err != nil {
		t.Fatalf("Marshal valid slot id: %v", err)
	}
	var decoded SlotID
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal valid slot id: %v", err)
	}
	if decoded != valid {
		t.Errorf("round-trip mismatch: got %q, want %q", decoded, valid)
	}

	// Invalid slot id should fail to unmarshal.
	if err := json.Unmarshal([]byte(`"slot/123"`), &decoded); err == nil {
		t.Error("expected error unmarshaling slot id with slash")
	}
	if err := json.Unmarshal([]byte(`""`), &decoded); err == nil {
		t.Error("expected error unmarshaling empty slot id")
	}
}

func TestTransferKindValidate(t *testing.T) {
	tests := []struct {
		name    string
		kind    TransferKind
		wantErr bool
	}{
		{name: "repo is valid", kind: TransferKindRepo, wantErr: false},
		{name: "artifact is valid", kind: TransferKindArtifact, wantErr: false},
		{name: "log is valid", kind: TransferKindLog, wantErr: false},
		{name: "cache is valid", kind: TransferKindCache, wantErr: false},
		{name: "empty is invalid", kind: "", wantErr: true},
		{name: "unknown kind is invalid", kind: "unknown", wantErr: true},
		{name: "typo is invalid", kind: "reop", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.kind.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("TransferKind.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTransferKindJSON(t *testing.T) {
	valid := TransferKindRepo
	data, err := json.Marshal(valid)
	if err != nil {
		t.Fatalf("Marshal valid kind: %v", err)
	}
	if string(data) != `"repo"` {
		t.Errorf("unexpected marshal: got %s, want %q", data, "repo")
	}
	var decoded TransferKind
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal valid kind: %v", err)
	}
	if decoded != valid {
		t.Errorf("round-trip mismatch: got %q, want %q", decoded, valid)
	}

	// Invalid kind should fail to unmarshal.
	if err := json.Unmarshal([]byte(`"invalid"`), &decoded); err == nil {
		t.Error("expected error unmarshaling invalid kind")
	}
}

func TestTransferStageValidate(t *testing.T) {
	tests := []struct {
		name    string
		stage   TransferStage
		wantErr bool
	}{
		{name: "plan is valid", stage: TransferStagePlan, wantErr: false},
		{name: "apply is valid", stage: TransferStageApply, wantErr: false},
		{name: "empty is valid (optional)", stage: "", wantErr: false},
		{name: "unknown stage is invalid", stage: "unknown", wantErr: true},
		{name: "typo is invalid", stage: "plna", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.stage.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("TransferStage.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTransferStageJSON(t *testing.T) {
	valid := TransferStagePlan
	data, err := json.Marshal(valid)
	if err != nil {
		t.Fatalf("Marshal valid stage: %v", err)
	}
	if string(data) != `"plan"` {
		t.Errorf("unexpected marshal: got %s, want %q", data, "plan")
	}
	var decoded TransferStage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal valid stage: %v", err)
	}
	if decoded != valid {
		t.Errorf("round-trip mismatch: got %q, want %q", decoded, valid)
	}

	// Invalid stage should fail to unmarshal.
	if err := json.Unmarshal([]byte(`"invalid"`), &decoded); err == nil {
		t.Error("expected error unmarshaling invalid stage")
	}

	// Empty stage should succeed (it's optional).
	if err := json.Unmarshal([]byte(`""`), &decoded); err != nil {
		t.Errorf("expected empty stage to succeed: %v", err)
	}
}
