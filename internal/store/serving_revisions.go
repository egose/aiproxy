package store

import "context"

type ServingRevisions struct {
	Catalog int64 `json:"catalog"`
	Keys    int64 `json:"keys"`
}

func (s *Store) ServingRevisions(ctx context.Context) (ServingRevisions, error) {
	var revisions ServingRevisions
	err := s.DB.NewSelect().Table("serving_revisions").Column("catalog", "keys").Where("singleton = TRUE").Scan(ctx, &revisions)
	return revisions, err
}
