package budget

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/budgets", h.handleBudgets)
}

func (h *Handler) handleBudgets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.createBudget(w, r)
	case http.MethodGet:
		h.getBudgetStatus(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) createBudget(w http.ResponseWriter, r *http.Request) {
	var req CreateBudgetRequest

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	budget, err := h.service.CreateBudget(r.Context(), req)
	if err != nil {
		var validationErr ValidationError
		if errors.As(err, &validationErr) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": validationErr.Message})
			return
		}

		var conflictErr ConflictError
		if errors.As(err, &conflictErr) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": conflictErr.Message})
			return
		}

		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusCreated, budget)
}

func (h *Handler) getBudgetStatus(w http.ResponseWriter, r *http.Request) {
	userIDValue := r.URL.Query().Get("user_id")
	if userIDValue == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}

	categoryIDValue := r.URL.Query().Get("category_id")
	if categoryIDValue == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "category_id is required"})
		return
	}

	periodStartValue := r.URL.Query().Get("period_start")
	if periodStartValue == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "period_start is required"})
		return
	}

	userID, err := strconv.ParseInt(userIDValue, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id must be a valid integer"})
		return
	}

	categoryID, err := strconv.ParseInt(categoryIDValue, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "category_id must be a valid integer"})
		return
	}

	periodStart, err := time.Parse("2006-01-02", periodStartValue)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "period_start must be in YYYY-MM-DD format"})
		return
	}

	status, err := h.service.GetBudgetStatus(r.Context(), userID, categoryID, periodStart)
	if err != nil {
		var validationErr ValidationError
		if errors.As(err, &validationErr) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": validationErr.Message})
			return
		}

		var notFoundErr NotFoundError
		if errors.As(err, &notFoundErr) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": notFoundErr.Message})
			return
		}

		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
