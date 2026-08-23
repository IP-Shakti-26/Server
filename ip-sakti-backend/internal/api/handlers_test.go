package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/heythisissud/ip-sakti-backend/internal/api"
	"github.com/heythisissud/ip-sakti-backend/internal/summary"
	"github.com/heythisissud/ip-sakti-backend/pkg/config"
)

func TestExamplesEndpoint(t *testing.T) {
	cfg := &config.Config{
		Env:            "test",
		AllowedOrigins: []string{"*"},
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	sum := summary.NewSummarizer(nil, logger)
	h := api.NewHandler(cfg, nil, nil, nil, nil, nil, sum, logger)
	router := api.NewRouter(h, cfg, logger)

	req := httptest.NewRequest("GET", "/api/v1/examples", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	var res struct {
		Examples []api.Example `json:"examples"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if len(res.Examples) != 4 {
		t.Fatalf("Expected 4 examples, got %d", len(res.Examples))
	}

	expectedIDs := []string{"ex_01", "ex_02", "ex_03", "ex_04"}
	for i, ex := range res.Examples {
		if ex.ID != expectedIDs[i] {
			t.Errorf("Example %d ID mismatch: expected %s, got %s", i, expectedIDs[i], ex.ID)
		}
		if ex.Title == "" || ex.Description == "" || len(ex.Tags) == 0 || ex.Complexity == "" {
			t.Errorf("Example %s has incomplete fields: %+v", ex.ID, ex)
		}
	}
}
