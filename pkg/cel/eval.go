// Package runtime provides CEL expression evaluation.
package cel

import (
	"errors"
	"fmt"
	"reflect"
	"sync"

	"github.com/google/cel-go/cel"
)

var (
	ErrValidationFailed = errors.New("validation rule failed")
	ErrTransitionFailed = errors.New("transition rule failed")
)

const (
	varThis    = "this"
	varSelf    = "self"
	varOldSelf = "oldSelf"
)

var (
	envCache  sync.Map
	progCache sync.Map
)

var (
	fieldTransitionActivationPool = sync.Pool{
		New: func() any {
			return &fieldTransitionActivation{}
		},
	}
	simpleValidateActivationPool = sync.Pool{
		New: func() any {
			return &simpleValidateActivation{}
		},
	}
	messageTransitionActivationPool = sync.Pool{
		New: func() any {
			return &messageTransitionActivation{}
		},
	}
)

var (
	validateEnvOnce sync.Once
	validateEnv     *cel.Env

	msgTransitionEnvOnce sync.Once
	msgTransitionEnv     *cel.Env
)

type fieldTransitionActivation struct {
	this    any
	oldSelf any
}

func (a *fieldTransitionActivation) ResolveName(name string) (any, bool) {
	switch name {
	case varThis:
		return a.this, true
	case varOldSelf:
		return a.oldSelf, true
	}
	return nil, false
}

func (a *fieldTransitionActivation) Parent() cel.Activation {
	return nil
}

type simpleValidateActivation struct {
	val any
}

func (a *simpleValidateActivation) ResolveName(name string) (any, bool) {
	switch name {
	case varThis, varSelf:
		return a.val, true
	}
	return nil, false
}

func (a *simpleValidateActivation) Parent() cel.Activation {
	return nil
}

type messageTransitionActivation struct {
	self    any
	oldSelf any
}

func (a *messageTransitionActivation) ResolveName(name string) (any, bool) {
	switch name {
	case varSelf:
		return a.self, true
	case varOldSelf:
		return a.oldSelf, true
	}
	return nil, false
}

func (a *messageTransitionActivation) Parent() cel.Activation {
	return nil
}

// EvalTransitionRule evaluates a field transition rule using 'this' and 'oldSelf'.
func EvalTransitionRule(expr string, newVal, oldVal any) error {
	prog, err := getOrCompileProgram(expr, newVal, oldVal)
	if err != nil {
		return fmt.Errorf("compile cel %q: %w", expr, err)
	}

	act := fieldTransitionActivationPool.Get().(*fieldTransitionActivation)
	act.this = newVal
	act.oldSelf = oldVal

	out, _, err := prog.Eval(act)

	act.this = nil
	act.oldSelf = nil
	fieldTransitionActivationPool.Put(act)

	if err != nil {
		return fmt.Errorf("eval cel %q: %w", expr, err)
	}
	if out.Value() != true {
		return ErrTransitionFailed
	}
	return nil
}

func getOrCompileProgram(expr string, newVal, oldVal any) (cel.Program, error) {
	thisType := reflect.TypeOf(newVal)
	oldType := reflect.TypeOf(oldVal)
	cacheKey := fmt.Sprintf("%s:%v:%v", expr, thisType, oldType)

	if prog, ok := progCache.Load(cacheKey); ok {
		return prog.(cel.Program), nil
	}

	env, err := createEnv(newVal, oldVal)
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

	progCache.Store(cacheKey, prog)
	return prog, nil
}

func createEnv(newVal, oldVal any) (*cel.Env, error) {
	thisType := goTypeToCelType(newVal)
	oldSelfType := goTypeToCelType(oldVal)
	cacheKey := fmt.Sprintf("field:%s:%s", thisType.String(), oldSelfType.String())

	if env, ok := envCache.Load(cacheKey); ok {
		return env.(*cel.Env), nil
	}

	env, err := cel.NewEnv(
		cel.Variable(varThis, thisType),
		cel.Variable(varOldSelf, oldSelfType),
	)
	if err != nil {
		return nil, fmt.Errorf("create env: %w", err)
	}

	envCache.Store(cacheKey, env)
	return env, nil
}

func goTypeToCelType(v any) *cel.Type {
	switch v.(type) {
	case string:
		return cel.StringType
	case int, int32, int64:
		return cel.IntType
	case uint, uint32, uint64:
		return cel.UintType
	case float32, float64:
		return cel.DoubleType
	case bool:
		return cel.BoolType
	case []byte:
		return cel.BytesType
	default:
		return cel.DynType
	}
}

// EvalValidateRule evaluates a validation rule using 'this' or 'self'.
func EvalValidateRule(expr string, val any) error {
	prog, err := getOrCompileValidateProgram(expr)
	if err != nil {
		return fmt.Errorf("compile cel %q: %w", expr, err)
	}

	act := simpleValidateActivationPool.Get().(*simpleValidateActivation)
	act.val = val

	out, _, err := prog.Eval(act)

	act.val = nil
	simpleValidateActivationPool.Put(act)

	if err != nil {
		return fmt.Errorf("eval cel %q: %w", expr, err)
	}
	if out.Value() != true {
		return ErrValidationFailed
	}
	return nil
}

func getOrCompileValidateProgram(expr string) (cel.Program, error) {
	cacheKey := "validate:" + expr

	if prog, ok := progCache.Load(cacheKey); ok {
		return prog.(cel.Program), nil
	}

	env, err := getValidateEnv()
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

	progCache.Store(cacheKey, prog)
	return prog, nil
}

func getValidateEnv() (*cel.Env, error) {
	var envErr error
	validateEnvOnce.Do(func() {
		validateEnv, envErr = cel.NewEnv(
			cel.Variable(varThis, cel.DynType),
			cel.Variable(varSelf, cel.DynType),
		)
	})
	if envErr != nil {
		return nil, fmt.Errorf("create validate env: %w", envErr)
	}
	return validateEnv, nil
}

// EvalMessageTransitionRule evaluates a message transition using 'self' and 'oldSelf'.
func EvalMessageTransitionRule(expr string, newMsg, oldMsg any) error {
	prog, err := getOrCompileMsgTransitionProgram(expr)
	if err != nil {
		return fmt.Errorf("compile cel %q: %w", expr, err)
	}

	act := messageTransitionActivationPool.Get().(*messageTransitionActivation)
	act.self = newMsg
	act.oldSelf = oldMsg

	out, _, err := prog.Eval(act)

	act.self = nil
	act.oldSelf = nil
	messageTransitionActivationPool.Put(act)

	if err != nil {
		return fmt.Errorf("eval cel %q: %w", expr, err)
	}
	if out.Value() != true {
		return ErrTransitionFailed
	}
	return nil
}

func getOrCompileMsgTransitionProgram(expr string) (cel.Program, error) {
	cacheKey := "msgtransition:" + expr

	if prog, ok := progCache.Load(cacheKey); ok {
		return prog.(cel.Program), nil
	}

	env, err := getMsgTransitionEnv()
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

	progCache.Store(cacheKey, prog)
	return prog, nil
}

func getMsgTransitionEnv() (*cel.Env, error) {
	var envErr error
	msgTransitionEnvOnce.Do(func() {
		msgTransitionEnv, envErr = cel.NewEnv(
			cel.Variable(varSelf, cel.DynType),
			cel.Variable(varOldSelf, cel.DynType),
		)
	})
	if envErr != nil {
		return nil, fmt.Errorf("create msg transition env: %w", envErr)
	}
	return msgTransitionEnv, nil
}
