package governor

import (
	"context"
)

type (
	// Injector is a dependency injector
	Injector interface {
		Get(key interface{}) interface{}
		Set(key, value interface{})
		Clone() Injector
	}

	govinjector struct {
		ctx context.Context
	}
)

func newInjector(ctx context.Context) Injector {
	return &govinjector{
		ctx: ctx,
	}
}

func (g *govinjector) Get(key interface{}) interface{} {
	return g.ctx.Value(key)
}

func (g *govinjector) Set(key, value interface{}) {
	g.ctx = context.WithValue(g.ctx, key, value)
}

func (g *govinjector) Clone() Injector {
	return &govinjector{
		ctx: g.ctx,
	}
}

// Injector gets a clone of the server injector instance
func (s *Server) Injector() Injector {
	return s.inj.Clone()
}
