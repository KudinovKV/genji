package stream

import (
	"fmt"

	"github.com/genjidb/genji/document"
	"github.com/genjidb/genji/sql/query/expr"
)

const (
	groupEnvKey = "_group"
	accEnvKey   = "_acc"
)

// An Operator is used to modify a stream.
// It takes an environment containing the current value as well as any other metadata
// created by other operatorsand returns a new environment which will be passed to the next operator.
// If it returns a nil environment, the env will be ignored.
// If it returns an error, the stream will be interrupted and that error will bubble up
// and returned by this function, unless that error is ErrStreamClosed, in which case
// the Iterate method will stop the iteration and return nil.
// Stream operators can be reused, and thus, any state or side effect should be kept within the Op closure
// unless the nature of the operator prevents that.
type Operator interface {
	Op() (OperatorFunc, error)
}

// An OperatorFunc is the function that will receive each value of the stream.
type OperatorFunc func(env *expr.Environment) (*expr.Environment, error)

// A MapOperator applies an expression on each value of the stream and returns a new value.
type MapOperator struct {
	E expr.Expr
}

// Map evaluates e on each value of the stream and outputs the result.
func Map(e expr.Expr) *MapOperator {
	return &MapOperator{E: e}
}

// Op implements the Operator interface.
func (m *MapOperator) Op() (OperatorFunc, error) {
	var newEnv expr.Environment

	return func(env *expr.Environment) (*expr.Environment, error) {
		v, err := m.E.Eval(env)
		if err != nil {
			return nil, err
		}

		newEnv.SetCurrentValue(v)
		newEnv.Outer = env
		return &newEnv, nil
	}, nil
}

func (m *MapOperator) String() string {
	return fmt.Sprintf("map(%s)", m.E)
}

// A FilterOperator filters values based on a given expression.
type FilterOperator struct {
	E expr.Expr
}

// Filter evaluates e for each incoming value and filters any value whose result is not truthy.
func Filter(e expr.Expr) *FilterOperator {
	return &FilterOperator{E: e}
}

// Op implements the Operator interface.
func (m *FilterOperator) Op() (OperatorFunc, error) {
	return func(env *expr.Environment) (*expr.Environment, error) {
		v, err := m.E.Eval(env)
		if err != nil {
			return nil, err
		}

		ok, err := v.IsTruthy()
		if err != nil {
			return nil, err
		}

		if !ok {
			return nil, nil
		}

		return env, nil
	}, nil
}

func (m *FilterOperator) String() string {
	return fmt.Sprintf("filter(%s)", m.E)
}

// A TakeOperator closes the stream after a certain number of values.
type TakeOperator struct {
	E expr.Expr
}

// Take closes the stream after n values have passed through the operator.
// n must evaluate to a number or to a value that can be converted to an integer.
func Take(n expr.Expr) *TakeOperator {
	return &TakeOperator{E: n}
}

// Op implements the Operator interface.
func (m *TakeOperator) Op() (OperatorFunc, error) {
	var n, count int64
	v, err := m.E.Eval(&expr.Environment{})
	if err != nil {
		return nil, err
	}
	if v.Type != document.IntegerValue {
		v, err = v.CastAsInteger()
		if err != nil {
			return nil, err
		}
	}
	n = v.V.(int64)

	return func(env *expr.Environment) (*expr.Environment, error) {
		if count < n {
			count++
			return env, nil
		}

		return nil, ErrStreamClosed
	}, nil
}

func (m *TakeOperator) String() string {
	return fmt.Sprintf("take(%s)", m.E)
}

// A SkipOperator skips the n first values of the stream.
type SkipOperator struct {
	E expr.Expr
}

// Skip ignores the first n values of the stream.
// n must evaluate to a number or to a value that can be converted to an integer.
func Skip(n expr.Expr) *SkipOperator {
	return &SkipOperator{E: n}
}

// Op implements the Operator interface.
func (m *SkipOperator) Op() (OperatorFunc, error) {
	var n, skipped int64
	v, err := m.E.Eval(&expr.Environment{})
	if err != nil {
		return nil, err
	}
	if v.Type != document.IntegerValue {
		v, err = v.CastAsInteger()
		if err != nil {
			return nil, err
		}
	}
	n = v.V.(int64)

	return func(env *expr.Environment) (*expr.Environment, error) {
		if skipped < n {
			skipped++
			return nil, nil
		}

		return env, nil
	}, nil
}

func (m *SkipOperator) String() string {
	return fmt.Sprintf("skip(%s)", m.E)
}

// A GroupByOperator applies an expression on each value of the stream and stores the result in the _group
// variable in the output stream.
type GroupByOperator struct {
	E expr.Expr
}

// GroupBy applies e on each value of the stream and stores the result in the _group
// variable in the output stream.
func GroupBy(e expr.Expr) *GroupByOperator {
	return &GroupByOperator{E: e}
}

// Op implements the Operator interface.
func (op *GroupByOperator) Op() (OperatorFunc, error) {
	var newEnv expr.Environment

	return func(env *expr.Environment) (*expr.Environment, error) {
		v, err := op.E.Eval(env)
		if err != nil {
			return nil, err
		}

		newEnv.Set(groupEnvKey, v)
		newEnv.Outer = env
		return &newEnv, nil
	}, nil
}

func (op *GroupByOperator) String() string {
	return fmt.Sprintf("groupBy(%s)", op.E)
}

// A ReduceOperator consumes the given stream and outputs one value per group.
// It reads the _group variable from the environment to determine with group
// to assign each value. If no _group variable is available, it will assume all
// values are part of the same group and reduce them into one value.
// To reduce incoming values, reduce
type ReduceOperator struct {
	Seed, Accumulator expr.Expr
	Stream            Stream
}

// Reduce consumes the incoming stream and outputs one value per group.
// It reads the _group variable from the environment to determine whitch group
// to assign each value. If no _group variable is available, it will assume all
// values are part of the same group and reduce them into one value.
// The seed is used to determine the initial value of the reduction. The initial value
// is stored in the _acc variable of the environment.
// The accumulator then takes the environment for each incoming value and is used to
// compute the new value of the _acc variable.
func Reduce(seed, accumulator expr.Expr) *ReduceOperator {
	return &ReduceOperator{Seed: seed, Accumulator: accumulator}
}

// Pipe stores s in the operator and return a new Stream with the reduce operator appended. It implements the Piper interface.
func (op *ReduceOperator) Pipe(s Stream) Stream {
	op.Stream = s

	return Stream{
		it: s,
		op: op,
	}
}

// Op implements the Operator interface.
func (op *ReduceOperator) Op() (OperatorFunc, error) {
	var newEnv expr.Environment

	seed, err := op.Seed.Eval(&newEnv)
	if err != nil {
		return nil, err
	}

	newEnv.Set(accEnvKey, seed)
	err = op.Stream.Iterate(func(env *expr.Environment) error {
		newEnv.Outer = env
		v, err := op.Accumulator.Eval(&newEnv)
		if err != nil {
			return err
		}

		newEnv.Set(accEnvKey, v)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return func(env *expr.Environment) (*expr.Environment, error) {
		v, _ := newEnv.Get(document.Path{document.PathFragment{FieldName: accEnvKey}})
		newEnv.SetCurrentValue(v)
		newEnv.Outer = env
		return &newEnv, nil
	}, nil
}

func (op *ReduceOperator) String() string {
	return fmt.Sprintf("reduce(%s, %s)", op.Seed, op.Accumulator)
}