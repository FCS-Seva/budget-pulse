package budget

import (
	"context"
	"time"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateBudget(ctx context.Context, req CreateBudgetRequest) (Budget, error) {
	if err := validateCreateBudgetRequest(req); err != nil {
		return Budget{}, err
	}

	req.PeriodStart = monthStart(req.PeriodStart)

	return s.repo.CreateBudget(ctx, req)
}

func (s *Service) GetBudgetStatus(ctx context.Context, userID, categoryID int64, periodStart time.Time) (BudgetStatus, error) {
	if userID <= 0 {
		return BudgetStatus{}, ValidationError{Message: "user_id must be greater than 0"}
	}

	if categoryID <= 0 {
		return BudgetStatus{}, ValidationError{Message: "category_id must be greater than 0"}
	}

	if periodStart.IsZero() {
		return BudgetStatus{}, ValidationError{Message: "period_start is required"}
	}

	return s.repo.GetBudgetStatus(ctx, userID, categoryID, monthStart(periodStart))
}
