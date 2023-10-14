package ratelimit

import (
	"context"

	"xorkevin.dev/governor"
)

type (
	Unrestricted struct{}
)

func (s *Unrestricted) Ratelimit(ctx context.Context, tags []Tag) error {
	return nil
}

func (s *Unrestricted) Subtree(prefix string) Limiter {
	return s
}

func (s *Unrestricted) BaseTagger() ReqTagger {
	return NoopTagger
}

func NoopTagger(c *governor.Context) []Tag {
	return nil
}
