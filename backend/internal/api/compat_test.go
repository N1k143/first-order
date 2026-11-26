package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompatibilityCategoryCollectionIsArray(t *testing.T) {
	canonical := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/categories" {
			t.Fatal("unexpected canonical route")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":"category-id","name":"Кафе"}],"total":1,"page":1,"limit":20}`))
	})
	rec := httptest.NewRecorder()
	(&Server{}).compatibility(canonical).ServeHTTP(rec, httptest.NewRequest("GET", "/categories", nil))
	var categories []map[string]any
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &categories) != nil || len(categories) != 1 || categories[0]["name"] != "Кафе" {
		t.Fatal("legacy category collection must be a directly iterable array")
	}
}
