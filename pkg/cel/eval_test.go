package cel

import (
	"errors"
	"testing"
)

func TestEvalTransitionRule(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		newVal  any
		oldVal  any
		wantErr error
	}{
		{
			name:    "int greater than passes",
			expr:    "this > oldSelf",
			newVal:  int64(10),
			oldVal:  int64(5),
			wantErr: nil,
		},
		{
			name:    "int greater than fails",
			expr:    "this > oldSelf",
			newVal:  int64(5),
			oldVal:  int64(10),
			wantErr: ErrTransitionFailed,
		},
		{
			name:    "int equal fails strict greater",
			expr:    "this > oldSelf",
			newVal:  int64(5),
			oldVal:  int64(5),
			wantErr: ErrTransitionFailed,
		},
		{
			name:    "int greater or equal passes on equal",
			expr:    "this >= oldSelf",
			newVal:  int64(5),
			oldVal:  int64(5),
			wantErr: nil,
		},
		{
			name:    "int greater or equal passes on greater",
			expr:    "this >= oldSelf",
			newVal:  int64(10),
			oldVal:  int64(5),
			wantErr: nil,
		},
		{
			name:    "string equality passes",
			expr:    "this == oldSelf",
			newVal:  "foo",
			oldVal:  "foo",
			wantErr: nil,
		},
		{
			name:    "string equality fails",
			expr:    "this == oldSelf",
			newVal:  "foo",
			oldVal:  "bar",
			wantErr: ErrTransitionFailed,
		},
		{
			name:    "string not equal passes",
			expr:    "this != oldSelf",
			newVal:  "foo",
			oldVal:  "bar",
			wantErr: nil,
		},
		{
			name:    "bool transition",
			expr:    "this || !oldSelf",
			newVal:  true,
			oldVal:  false,
			wantErr: nil,
		},
		{
			name:    "uint comparison",
			expr:    "this >= oldSelf",
			newVal:  uint64(100),
			oldVal:  uint64(50),
			wantErr: nil,
		},
		{
			name:    "double comparison",
			expr:    "this > oldSelf",
			newVal:  3.14,
			oldVal:  2.71,
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EvalTransitionRule(tt.expr, tt.newVal, tt.oldVal)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("EvalTransitionRule() unexpected error = %v", err)
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("EvalTransitionRule() error = %v, want %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestEvalTransitionRule_CompileError(t *testing.T) {
	err := EvalTransitionRule("invalid syntax +++", int64(1), int64(1))
	if err == nil {
		t.Error("expected compile error for invalid expression")
	}
	if errors.Is(err, ErrTransitionFailed) {
		t.Error("compile error should not be ErrTransitionFailed")
	}
}

func TestEvalValidateRule(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		val     any
		wantErr error
	}{
		{
			name:    "int positive passes",
			expr:    "this > 0",
			val:     int64(5),
			wantErr: nil,
		},
		{
			name:    "int positive fails",
			expr:    "this > 0",
			val:     int64(-1),
			wantErr: ErrValidationFailed,
		},
		{
			name:    "string not empty passes",
			expr:    "this != ''",
			val:     "hello",
			wantErr: nil,
		},
		{
			name:    "string not empty fails",
			expr:    "this != ''",
			val:     "",
			wantErr: ErrValidationFailed,
		},
		{
			name:    "self alias works",
			expr:    "self > 0",
			val:     int64(10),
			wantErr: nil,
		},
		{
			name:    "bool true passes",
			expr:    "this == true",
			val:     true,
			wantErr: nil,
		},
		{
			name:    "bool false fails true check",
			expr:    "this == true",
			val:     false,
			wantErr: ErrValidationFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EvalValidateRule(tt.expr, tt.val)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("EvalValidateRule() unexpected error = %v", err)
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("EvalValidateRule() error = %v, want %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestEvalValidateRule_CompileError(t *testing.T) {
	err := EvalValidateRule("broken expr !!!", int64(1))
	if err == nil {
		t.Error("expected compile error for invalid expression")
	}
	if errors.Is(err, ErrValidationFailed) {
		t.Error("compile error should not be ErrValidationFailed")
	}
}

func TestEvalMessageTransitionRule(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		newVal  any
		oldVal  any
		wantErr error
	}{
		{
			name:    "self greater than oldSelf passes",
			expr:    "self > oldSelf",
			newVal:  int64(10),
			oldVal:  int64(5),
			wantErr: nil,
		},
		{
			name:    "self greater than oldSelf fails",
			expr:    "self > oldSelf",
			newVal:  int64(5),
			oldVal:  int64(10),
			wantErr: ErrTransitionFailed,
		},
		{
			name:    "equality check passes",
			expr:    "self == oldSelf",
			newVal:  "immutable",
			oldVal:  "immutable",
			wantErr: nil,
		},
		{
			name:    "equality check fails",
			expr:    "self == oldSelf",
			newVal:  "new",
			oldVal:  "old",
			wantErr: ErrTransitionFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EvalMessageTransitionRule(tt.expr, tt.newVal, tt.oldVal)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("EvalMessageTransitionRule() unexpected error = %v", err)
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("EvalMessageTransitionRule() error = %v, want %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestGoTypeToCelType(t *testing.T) {
	tests := []struct {
		name     string
		val      any
		wantType string
	}{
		{"string", "hello", "string"},
		{"int", 42, "int"},
		{"int32", int32(42), "int"},
		{"int64", int64(42), "int"},
		{"uint", uint(42), "uint"},
		{"uint32", uint32(42), "uint"},
		{"uint64", uint64(42), "uint"},
		{"float32", float32(3.14), "double"},
		{"float64", float64(3.14), "double"},
		{"bool", true, "bool"},
		{"bytes", []byte("hello"), "bytes"},
		{"unknown struct", struct{ X int }{X: 1}, "dyn"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := goTypeToCelType(tt.val)
			if got.String() != tt.wantType {
				t.Errorf("goTypeToCelType(%T) = %v, want %v", tt.val, got.String(), tt.wantType)
			}
		})
	}
}

func TestProgramCaching(t *testing.T) {
	expr := "this > oldSelf"
	newVal := int64(10)
	oldVal := int64(5)

	// first call compiles
	err := EvalTransitionRule(expr, newVal, oldVal)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// second call should use cache
	err = EvalTransitionRule(expr, newVal, oldVal)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	// different values, same types, same expression should use cache
	err = EvalTransitionRule(expr, int64(20), int64(15))
	if err != nil {
		t.Fatalf("third call failed: %v", err)
	}
}
