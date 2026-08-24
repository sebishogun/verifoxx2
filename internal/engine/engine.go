package engine

import (
	"errors"
	"fmt"

	"github.com/sebishogun/verifoxx2/internal/adapters/jsonio"
	"github.com/sebishogun/verifoxx2/internal/ast"
	policycompile "github.com/sebishogun/verifoxx2/internal/compile"
	"github.com/sebishogun/verifoxx2/internal/eval"
	"github.com/sebishogun/verifoxx2/internal/input"
	"github.com/sebishogun/verifoxx2/internal/program"
	"github.com/sebishogun/verifoxx2/internal/result"
)

// Engine owns a validated immutable program. Sessions may share an Engine,
// but each Session must remain private to one sequential worker.
type Engine struct {
	compiled  program.Program
	evaluator eval.Evaluator
	limits    eval.Limits
}

type Session struct {
	engine  *Engine
	builder eval.Builder
	batch   eval.Batch
	context eval.Context
	numeric result.Batch
	output  jsonio.OutputPack
}

type InputError struct {
	Err error
}

func (err *InputError) Error() string { return "build evaluation batch: " + err.Err.Error() }
func (err *InputError) Unwrap() error { return err.Err }

func IsInputError(err error) bool {
	var inputError *InputError
	return errors.As(err, &inputError)
}

func New(compiled program.Program, limits eval.Limits) (*Engine, error) {
	if err := compiled.Validate(); err != nil {
		return nil, fmt.Errorf("create engine: %w", err)
	}
	return newValidated(compiled, limits), nil
}

func Compile(source ast.Policy, limits eval.Limits) (*Engine, []policycompile.Diagnostic) {
	compiled, diagnostics := policycompile.Compile(source)
	if len(diagnostics) != 0 {
		return nil, diagnostics
	}
	return newValidated(compiled, limits), nil
}

func newValidated(compiled program.Program, limits eval.Limits) *Engine {
	engine := &Engine{compiled: compiled, limits: limits}
	engine.evaluator = eval.NewEvaluator(&engine.compiled)
	return engine
}

func (engine *Engine) Name() string    { return engine.compiled.Name }
func (engine *Engine) Version() string { return engine.compiled.Version }

func (engine *Engine) NewSession() *Session {
	if engine == nil {
		return &Session{}
	}
	return &Session{
		engine:  engine,
		builder: eval.NewBuilder(&engine.compiled, engine.limits),
	}
}

func (session *Session) Evaluate(requests []input.Request, evidence []input.Evidence) (*jsonio.OutputPack, error) {
	if session == nil || session.engine == nil {
		return nil, fmt.Errorf("evaluation session has no engine")
	}
	if err := session.builder.BuildInto(&session.batch, requests, evidence); err != nil {
		return nil, &InputError{Err: err}
	}
	rows := len(requests)
	requirementCapacity, ok := checkedProduct(rows, len(session.engine.compiled.RequirementSymbols))
	if !ok {
		return nil, fmt.Errorf("requirement result capacity overflows addressable memory")
	}
	remediationCapacity, ok := checkedProduct(rows, len(session.engine.compiled.Remediations))
	if !ok {
		return nil, fmt.Errorf("remediation result capacity overflows addressable memory")
	}
	session.context.Ensure(session.engine.compiled, session.batch.Rows)
	session.numeric.Ensure(session.batch.Rows, requirementCapacity, len(session.batch.EvidenceRefs), remediationCapacity)
	if err := session.engine.evaluator.EvaluateInto(&session.context, session.batch, &session.numeric); err != nil {
		return nil, fmt.Errorf("evaluate batch: %w", err)
	}
	if err := jsonio.MaterializeInto(&session.output, session.engine.compiled, session.batch, session.numeric, requests, evidence); err != nil {
		return nil, fmt.Errorf("materialize results: %w", err)
	}
	return &session.output, nil
}

func checkedProduct(left, right int) (int, bool) {
	if left == 0 || right == 0 {
		return 0, true
	}
	maxInt := int(^uint(0) >> 1)
	if left > maxInt/right {
		return 0, false
	}
	return left * right, true
}
