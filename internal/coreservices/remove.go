package services

import (
	"context"

	"github.com/hmwassim/debforge/internal/services/self"
)

type SelfRemoveService struct {
	remover *self.Remover
}

func NewSelfRemoveService(remover *self.Remover) *SelfRemoveService {
	return &SelfRemoveService{remover: remover}
}

func (s *SelfRemoveService) Run(ctx context.Context, selection string) error {
	return s.remover.Remove(ctx, selection)
}
