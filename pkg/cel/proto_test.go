package cel

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestEvalProtoValidateRule_NilMessage(t *testing.T) {
	err := EvalProtoValidateRule("true", nil)
	if err != nil {
		t.Errorf("expected nil error for nil message, got %v", err)
	}
}

func TestEvalProtoValidateRule(t *testing.T) {
	// cel treats duration as a native duration type, use comparison operators
	tests := []struct {
		name    string
		expr    string
		msg     proto.Message
		wantErr error
	}{
		{
			name:    "duration greater than zero passes",
			expr:    "self > duration('0s')",
			msg:     durationpb.New(5000000000), // 5 seconds
			wantErr: nil,
		},
		{
			name:    "duration greater than zero fails",
			expr:    "self > duration('0s')",
			msg:     durationpb.New(-1000000000), // -1 second
			wantErr: ErrValidationFailed,
		},
		{
			name:    "duration less than hour passes",
			expr:    "self < duration('1h')",
			msg:     durationpb.New(60000000000), // 60 seconds
			wantErr: nil,
		},
		{
			name:    "duration less than minute fails",
			expr:    "self < duration('1m')",
			msg:     durationpb.New(120000000000), // 120 seconds
			wantErr: ErrValidationFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EvalProtoValidateRule(tt.expr, tt.msg)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("EvalProtoValidateRule() unexpected error = %v", err)
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("EvalProtoValidateRule() error = %v, want %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestEvalProtoValidateRule_CompileError(t *testing.T) {
	msg := durationpb.New(1000000000)
	err := EvalProtoValidateRule("broken !!!", msg)
	if err == nil {
		t.Error("expected compile error for invalid expression")
	}
	if errors.Is(err, ErrValidationFailed) {
		t.Error("compile error should not be ErrValidationFailed")
	}
}

func TestEvalProtoTransitionRule_NilMessages(t *testing.T) {
	tests := []struct {
		name   string
		newMsg proto.Message
		oldMsg proto.Message
	}{
		{"nil new", nil, wrapperspb.Int32(1)},
		{"nil old", wrapperspb.Int32(1), nil},
		{"both nil", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EvalProtoTransitionRule("true", tt.newMsg, tt.oldMsg)
			if err != nil {
				t.Errorf("expected nil error for nil message, got %v", err)
			}
		})
	}
}

func TestEvalProtoTransitionRule(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		newMsg  proto.Message
		oldMsg  proto.Message
		wantErr error
	}{
		{
			name:    "duration increasing passes",
			expr:    "self > oldSelf",
			newMsg:  durationpb.New(10000000000), // 10 seconds
			oldMsg:  durationpb.New(5000000000),  // 5 seconds
			wantErr: nil,
		},
		{
			name:    "duration increasing fails",
			expr:    "self > oldSelf",
			newMsg:  durationpb.New(5000000000),  // 5 seconds
			oldMsg:  durationpb.New(10000000000), // 10 seconds
			wantErr: ErrTransitionFailed,
		},
		{
			name:    "duration immutable passes",
			expr:    "self == oldSelf",
			newMsg:  durationpb.New(30000000000), // 30 seconds
			oldMsg:  durationpb.New(30000000000), // 30 seconds
			wantErr: nil,
		},
		{
			name:    "duration immutable fails",
			expr:    "self == oldSelf",
			newMsg:  durationpb.New(60000000000), // 60 seconds
			oldMsg:  durationpb.New(30000000000), // 30 seconds
			wantErr: ErrTransitionFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := EvalProtoTransitionRule(tt.expr, tt.newMsg, tt.oldMsg)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("EvalProtoTransitionRule() unexpected error = %v", err)
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("EvalProtoTransitionRule() error = %v, want %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestEvalProtoTransitionRule_CompileError(t *testing.T) {
	newMsg := durationpb.New(10000000000)
	oldMsg := durationpb.New(5000000000)
	err := EvalProtoTransitionRule("invalid syntax +++", newMsg, oldMsg)
	if err == nil {
		t.Error("expected compile error for invalid expression")
	}
	if errors.Is(err, ErrTransitionFailed) {
		t.Error("compile error should not be ErrTransitionFailed")
	}
}

func TestProtoEnvCaching(t *testing.T) {
	msg1 := durationpb.New(5000000000)
	msg2 := durationpb.New(10000000000)

	// first call compiles environment
	err := EvalProtoValidateRule("self > duration('0s')", msg1)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// second call with same type should use cached env
	err = EvalProtoValidateRule("self > duration('0s')", msg2)
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	// different expression, same type, should use cached env
	err = EvalProtoValidateRule("self < duration('1h')", msg1)
	if err != nil {
		t.Fatalf("third call failed: %v", err)
	}
}

func TestProtoProgramCaching(t *testing.T) {
	expr := "self > oldSelf"
	newMsg := durationpb.New(10000000000)
	oldMsg := durationpb.New(5000000000)

	// first call compiles
	err := EvalProtoTransitionRule(expr, newMsg, oldMsg)
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// same expression, different values should use cached program
	err = EvalProtoTransitionRule(expr, durationpb.New(20000000000), durationpb.New(15000000000))
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
}
