package ledger

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/transactions", h.handleTransactions)
}

func (h *Handler) handleTransactions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.createTransaction(w, r)
	case http.MethodGet:
		h.listTransactions(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (h *Handler) createTransaction(w http.ResponseWriter, r *http.Request) {
	var req CreateTransactionRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}

	tx, err := h.service.CreateTransaction(r.Context(), req)
	if err != nil {
		var validationErr ValidationError
		if errors.As(err, &validationErr) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": validationErr.Message})
			return
		}

		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusCreated, tx)
}

func (h *Handler) listTransactions(w http.ResponseWriter, r *http.Request) {
	userIDValue := r.URL.Query().Get("user_id")
	if userIDValue == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}

	userID, err := strconv.ParseInt(userIDValue, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id must be a valid integer"})
		return
	}

	items, err := h.service.ListTransactions(r.Context(), userID)
	if err != nil {
		var validationErr ValidationError
		if errors.As(err, &validationErr) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": validationErr.Message})
			return
		}

		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, items)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
