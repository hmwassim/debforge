package services

import (
	"context"

	"github.com/hmwassim/debforge/internal/services/self"
	"github.com/hmwassim/debforge/internal/ports"
)

type UpdateService struct {
	updater self.SelfUpdater
	logger  ports.UI
}

func NewUpdateService(updater self.SelfUpdater, logger ports.UI) *UpdateService {
	return &UpdateService{updater: updater, logger: logger}
}

func (s *UpdateService) Run(ctx context.Context) error {
	return s.updater.Update(ctx)
}
