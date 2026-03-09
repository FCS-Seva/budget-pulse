package ledger

import "context"

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateTransaction(ctx context.Context, req CreateTransactionRequest) (Transaction, error) {
	if err := validateCreateTransactionRequest(req); err != nil {
		return Transaction{}, err
	}

	return s.repo.CreateTransaction(ctx, req)
}

func (s *Service) ListTransactions(ctx context.Context, userID int64) ([]Transaction, error) {
	if userID <= 0 {
		return nil, ValidationError{Message: "user_id must be greater than 0"}
	}

	return s.repo.ListTransactions(ctx, userID)
}

func (s *Service) CreateCategory(ctx context.Context, req CreateCategoryRequest) (Category, error) {
	if err := validateCreateCategoryRequest(req); err != nil {
		return Category{}, err
	}

	return s.repo.CreateCategory(ctx, req)
}

func (s *Service) ListCategories(ctx context.Context, userID int64) ([]Category, error) {
	if userID <= 0 {
		return nil, ValidationError{Message: "user_id must be greater than 0"}
	}

	return s.repo.ListCategories(ctx, userID)
}
