package knowledgebase

import (
	"context"

	"github.com/doubletabai/doubletab/pkg/vector"
)

const (
	sampleOtherDB = `Other databases support.
Currently only PostgreSQL DB is supported. We'll be adding support for other databases in the future. You're welcome
to suggest a database you'd like to see supported by DoubleTab by starting a discussion on GitHub:
https://github.com/doubletabai/doubletab/discussions/categories/ideas.
`
)

func Populate(ctx context.Context, db *vector.KnowledgeService) error {
	if err := db.Store(ctx, sampleOtherDB); err != nil {
		return err
	}

	return nil
}
