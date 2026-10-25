package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nicheinc/expect"
	"go.uber.org/zap"
)

func Test_app_getInstanceEndpoint(t *testing.T) {
	t.Run("WithTermsURL", func(t *testing.T) {
		a := &app{logger: zap.NewNop(), instanceDomain: "example.com", instanceName: "Example", termsURL: "https://example.com/terms"}
		recorder := httptest.NewRecorder()
		a.routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/instance", nil))

		var body map[string]any
		expect.ErrorNil(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		expect.Equal[any](t, body["termsUrl"], "https://example.com/terms")
	})
	t.Run("Success", func(t *testing.T) {
		a := &app{logger: zap.NewNop(), instanceDomain: "example.com", instanceName: "Example"}
		recorder := httptest.NewRecorder()
		a.routes().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/instance", nil))

		var body map[string]any
		expect.ErrorNil(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		expect.Equal(t, recorder.Code, http.StatusOK)
		expect.Equal(t, body, map[string]any{"domain": "example.com", "name": "Example", "version": "dev"})
	})
}
