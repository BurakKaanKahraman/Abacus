package handler

import (
	"net/http"

	"github.com/BurakKaanKahraman/abacus/backend/internal/domain"
	"github.com/BurakKaanKahraman/abacus/backend/internal/httpx"
	"github.com/BurakKaanKahraman/abacus/backend/internal/usecase"
)

// CalculateHandler serves POST /api/v1/calculate.
type CalculateHandler struct {
	calculator *usecase.Calculator
}

// NewCalculateHandler wires the handler to the calculator usecase.
func NewCalculateHandler(calculator *usecase.Calculator) *CalculateHandler {
	return &CalculateHandler{calculator: calculator}
}

// Handle decodes the request, evaluates it and renders the result. Both
// payload shapes are accepted: a raw `expression` string or an `operation`
// with an `operands` array.
func (h *CalculateHandler) Handle(w http.ResponseWriter, r *http.Request) {
	var request domain.CalculateRequest
	if err := decodeJSON(r, &request); err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}

	result, err := h.calculator.Calculate(request)
	if err != nil {
		httpx.WriteProblem(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, domain.CalculateResponse{
		Expression: result.Expression,
		Result:     result.Result,
		Formatted:  result.Formatted,
		Timestamp:  httpx.Now(),
	})
}
