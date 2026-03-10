package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateTransaction(ctx context.Context, req CreateTransactionRequest, idempotencyKey string) (Transaction, bool, error) {
	req = normalizeCreateTransactionRequest(req)

	if err := validateCreateTransactionRequest(req); err != nil {
		return Transaction{}, false, err
	}

	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return Transaction{}, false, ValidationError{Message: "Idempotency-Key header is required"}
	}

	requestHash, err := makeRequestHash(req)
	if err != nil {
		return Transaction{}, false, err
	}

	return s.repo.CreateTransaction(ctx, req, idempotencyKey, requestHash)
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

func normalizeCreateTransactionRequest(req CreateTransactionRequest) CreateTransactionRequest {
	req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))

	if req.Merchant != nil {
		v := strings.TrimSpace(*req.Merchant)
		if v == "" {
			req.Merchant = nil
		} else {
			req.Merchant = &v
		}
	}

	return req
}

func makeRequestHash(req CreateTransactionRequest) (string, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
