package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nicheinc/expect"
	"go.uber.org/zap"
)

func Test_app_cors(t *testing.T) {
	const allowedOrigin = "http://localhost:8081"

	type testCase struct {
		origins          []string
		method           string
		headers          map[string]string
		expectedStatus   int
		expectedCalled   bool
		expectedOrigin   string
		expectedMethods  string
		expectedAllowHdr string
		expectedVary     string
	}
	run := func(name string, testCase testCase) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			t.Helper()
			called := false
			a := &app{logger: zap.NewNop(), corsOrigins: testCase.origins}
			handler := a.cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))

			request := httptest.NewRequest(testCase.method, "/", nil)
			for k, v := range testCase.headers {
				request.Header.Set(k, v)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)

			header := recorder.Header()
			expect.Equal(t, recorder.Code, testCase.expectedStatus)
			expect.Equal(t, called, testCase.expectedCalled)
			expect.Equal(t, header.Get("Access-Control-Allow-Origin"), testCase.expectedOrigin)
			expect.Equal(t, header.Get("Access-Control-Allow-Methods"), testCase.expectedMethods)
			expect.Equal(t, header.Get("Access-Control-Allow-Headers"), testCase.expectedAllowHdr)
			expect.Equal(t, header.Get("Vary"), testCase.expectedVary)
			expect.Equal(t, header.Get("Access-Control-Allow-Credentials"), "")
		})
	}

	preflight := func(origin string) map[string]string {
		return map[string]string{"Origin": origin, "Access-Control-Request-Method": "POST"}
	}

	run("NoOrigin", testCase{
		origins:        []string{allowedOrigin},
		method:         http.MethodGet,
		expectedStatus: http.StatusOK,
		expectedCalled: true,
	})
	run("DisallowedOrigin", testCase{
		origins:        []string{allowedOrigin},
		method:         http.MethodGet,
		headers:        map[string]string{"Origin": "http://evil.example"},
		expectedStatus: http.StatusOK,
		expectedCalled: true,
	})
	run("PreflightDisallowed", testCase{
		origins:        []string{allowedOrigin},
		method:         http.MethodOptions,
		headers:        preflight("http://evil.example"),
		expectedStatus: http.StatusForbidden,
	})
	run("PreflightAllowed", testCase{
		origins:          []string{allowedOrigin},
		method:           http.MethodOptions,
		headers:          preflight(allowedOrigin),
		expectedStatus:   http.StatusNoContent,
		expectedOrigin:   allowedOrigin,
		expectedMethods:  "GET, POST, PUT, PATCH, DELETE, OPTIONS",
		expectedAllowHdr: "Authorization, Content-Type",
		expectedVary:     "Origin",
	})
	run("Wildcard", testCase{
		origins:        []string{"*"},
		method:         http.MethodGet,
		headers:        map[string]string{"Origin": "http://anything.example"},
		expectedStatus: http.StatusOK,
		expectedCalled: true,
		expectedOrigin: "http://anything.example",
		expectedVary:   "Origin",
	})
	run("AllowedOrigin", testCase{
		origins:        []string{allowedOrigin},
		method:         http.MethodGet,
		headers:        map[string]string{"Origin": allowedOrigin},
		expectedStatus: http.StatusOK,
		expectedCalled: true,
		expectedOrigin: allowedOrigin,
		expectedVary:   "Origin",
	})
}

func Test_app_recoverPanics(t *testing.T) {
	a := &app{logger: zap.NewNop()}

	t.Run("Panic", func(t *testing.T) {
		handler := a.recoverPanics(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

		var body map[string]any
		expect.ErrorNil(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		expect.Equal(t, recorder.Code, http.StatusInternalServerError)
		expect.Equal(t, body, map[string]any{"error": "internal server error"})
	})
	t.Run("NoPanic", func(t *testing.T) {
		handler := a.recoverPanics(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) }))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		expect.Equal(t, recorder.Code, http.StatusTeapot)
	})
}
