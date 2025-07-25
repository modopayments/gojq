package gojq

import (
	"context"
	"sync"
)

type env struct {
	pc        int
	stack     *stack
	paths     *stack
	scopes    *scopeStack
	values    []any
	codes     []*code
	codeinfos []codeinfo
	forks     []fork
	backtrack bool
	offset    int
	expdepth  int
	label     int
	args      [32]any // len(env.args) > maxarity
	ctx       context.Context
}

func (env *env) reset(ctx context.Context) {
	env.pc = 0
	env.stack.reset()
	env.paths.reset()
	env.scopes.reset()
	env.values = env.values[:0]
	env.codes = env.codes[:0]
	env.codeinfos = env.codeinfos[:0]
	env.forks = env.forks[:0]
	env.backtrack = false
	env.offset = 0
	env.expdepth = 0
	env.label = 0
	env.ctx = ctx
}

var envPool sync.Pool

func init() {
	envPool.New = func() any {
		return &env{
			stack:  newStack(),
			paths:  newStack(),
			scopes: newScopeStack(),
		}
	}
}

func newEnv(ctx context.Context) *env {
	e := envPool.Get().(*env)
	e.reset(ctx)
	return e
}

type scope struct {
	id         int
	offset     int
	pc         int
	saveindex  int
	outerindex int
}

type fork struct {
	pc         int
	stackindex int
	stacklimit int
	scopeindex int
	scopelimit int
	pathindex  int
	pathlimit  int
	expdepth   int
}
