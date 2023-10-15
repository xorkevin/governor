package ratelimit

import (
	"context"

	"xorkevin.dev/governor"
)

type (
	Unlimited struct{}
)

func (s Unlimited) Ratelimit(ctx context.Context, tags []Tag) error {
	return nil
}

func (s Unlimited) Subtree(prefix string) Limiter {
	return s
}

func (s Unlimited) BaseTagger() ReqTagger {
	return NoopReqTagger
}

func NoopReqTagger(c *governor.Context) []Tag {
	return nil
}
