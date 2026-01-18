package cel

import (
	"fmt"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var (
	protoEnvCache     sync.Map
	protoProgramCache sync.Map
)

var (
	validateActivationPool = sync.Pool{
		New: func() any {
			return &validateActivation{}
		},
	}
	transitionActivationPool = sync.Pool{
		New: func() any {
			return &transitionActivation{}
		},
	}
)

type validateActivation struct {
	self proto.Message
}

func (a *validateActivation) ResolveName(name string) (any, bool) {
	if name == varSelf {
		return a.self, true
	}
	return nil, false
}

func (a *validateActivation) Parent() cel.Activation {
	return nil
}

type transitionActivation struct {
	self    proto.Message
	oldSelf proto.Message
}

func (a *transitionActivation) ResolveName(name string) (any, bool) {
	switch name {
	case varSelf:
		return a.self, true
	case varOldSelf:
		return a.oldSelf, true
	}
	return nil, false
}

func (a *transitionActivation) Parent() cel.Activation {
	return nil
}

// EvalProtoValidateRule evaluates a validation rule using 'self'.
func EvalProtoValidateRule(expr string, msg proto.Message) error {
	if msg == nil {
		return nil
	}

	prog, err := getOrCompileProtoProgram(expr, msg, false)
	if err != nil {
		return fmt.Errorf("compile cel: %w", err)
	}

	act := validateActivationPool.Get().(*validateActivation)
	act.self = msg

	out, _, err := prog.Eval(act)

	act.self = nil
	validateActivationPool.Put(act)

	if err != nil {
		return fmt.Errorf("eval cel %q: %w", expr, err)
	}
	if out.Value() != true {
		return ErrValidationFailed
	}
	return nil
}

// EvalProtoTransitionRule evaluates a transition rule using 'self' and 'oldSelf'.
func EvalProtoTransitionRule(expr string, newMsg, oldMsg proto.Message) error {
	if newMsg == nil || oldMsg == nil {
		return nil
	}

	prog, err := getOrCompileProtoProgram(expr, newMsg, true)
	if err != nil {
		return fmt.Errorf("compile cel: %w", err)
	}

	act := transitionActivationPool.Get().(*transitionActivation)
	act.self = newMsg
	act.oldSelf = oldMsg

	out, _, err := prog.Eval(act)

	act.self = nil
	act.oldSelf = nil
	transitionActivationPool.Put(act)

	if err != nil {
		return fmt.Errorf("eval cel %q: %w", expr, err)
	}
	if out.Value() != true {
		return ErrTransitionFailed
	}
	return nil
}

// EvalProtoFieldTransitionRule evaluates a field transition using 'this' and 'oldSelf'.
func EvalProtoFieldTransitionRule(expr string, newMsg, oldMsg proto.Message, fieldName string) error {
	if newMsg == nil || oldMsg == nil {
		return nil
	}

	newReflect := newMsg.ProtoReflect()
	oldReflect := oldMsg.ProtoReflect()

	fd := newReflect.Descriptor().Fields().ByName(protoreflect.Name(fieldName))
	if fd == nil {
		return fmt.Errorf("field %q not found", fieldName)
	}

	newVal := newReflect.Get(fd)
	oldVal := oldReflect.Get(fd)

	return EvalTransitionRule(expr, protoValueToGo(newVal, fd), protoValueToGo(oldVal, fd))
}

func protoValueToGo(v protoreflect.Value, fd protoreflect.FieldDescriptor) any {
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return v.Bool()
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return int32(v.Int())
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return v.Int()
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return uint32(v.Uint())
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return v.Uint()
	case protoreflect.FloatKind:
		return float32(v.Float())
	case protoreflect.DoubleKind:
		return v.Float()
	case protoreflect.StringKind:
		return v.String()
	case protoreflect.BytesKind:
		return v.Bytes()
	case protoreflect.EnumKind:
		return int32(v.Enum())
	case protoreflect.MessageKind:
		return v.Message().Interface()
	default:
		return v.Interface()
	}
}

func getOrCompileProtoProgram(expr string, msg proto.Message, hasOldSelf bool) (cel.Program, error) {
	msgDesc := msg.ProtoReflect().Descriptor()
	msgName := string(msgDesc.FullName())

	cacheKey := msgName + ":" + expr
	if hasOldSelf {
		cacheKey += ":transition"
	}

	if prog, ok := protoProgramCache.Load(cacheKey); ok {
		return prog.(cel.Program), nil
	}

	env, err := getOrCreateProtoEnv(msg, hasOldSelf)
	if err != nil {
		return nil, err
	}

	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compile: %w", issues.Err())
	}

	prog, err := env.Program(ast,
		cel.EvalOptions(cel.OptOptimize),
		cel.OptimizeRegex(),
	)
	if err != nil {
		return nil, fmt.Errorf("program: %w", err)
	}

	protoProgramCache.Store(cacheKey, prog)
	return prog, nil
}

func getOrCreateProtoEnv(msg proto.Message, hasOldSelf bool) (*cel.Env, error) {
	msgDesc := msg.ProtoReflect().Descriptor()
	msgName := string(msgDesc.FullName())

	cacheKey := msgName
	if hasOldSelf {
		cacheKey += ":transition"
	}

	if env, ok := protoEnvCache.Load(cacheKey); ok {
		return env.(*cel.Env), nil
	}

	opts := []cel.EnvOption{
		cel.Types(msg),
		ext.Strings(),
		ext.Encoders(),
	}

	msgType := cel.ObjectType(msgName)
	opts = append(opts, cel.Variable(varSelf, msgType))
	if hasOldSelf {
		opts = append(opts, cel.Variable(varOldSelf, msgType))
	}

	env, err := cel.NewEnv(opts...)
	if err != nil {
		return nil, fmt.Errorf("create proto env: %w", err)
	}

	protoEnvCache.Store(cacheKey, env)
	return env, nil
}
