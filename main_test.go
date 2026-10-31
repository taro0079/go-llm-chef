package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/taro0079/go-llm-chef/internal/repository"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// Each test gets its own database; app.db and .env are never used.
func testRepository(t *testing.T) (*repository.IngredientRepository, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE ingredients (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	return repository.NewIngredientRepository(db), db
}

func seedIngredient(t *testing.T, repo *repository.IngredientRepository, name string) *repository.Ingredient {
	t.Helper()
	ing, err := repo.Create(context.Background(), &repository.Ingredient{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	return ing
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testClient(t *testing.T, transport roundTripFunc) openai.Client {
	t.Helper()
	if transport == nil {
		transport = func(r *http.Request) (*http.Response, error) {
			t.Errorf("unexpected API request: %s %s", r.Method, r.URL)
			return nil, errors.New("unexpected API request")
		}
	}
	return openai.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL("https://recipe-api.invalid/v1/"),
		option.WithMaxRetries(0),
		option.WithHTTPClient(&http.Client{Transport: transport}),
	)
}

func serveForm(handler http.Handler, method, path string, values url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func TestIngredientHTTPFlow(t *testing.T) {
	repo, _ := testRepository(t)
	handler := newHandler(repo, testClient(t, nil))
	for _, tc := range []struct {
		name, method, path string
		form               url.Values
		contains, excludes []string
		count              int
	}{
		{name: "home", method: "GET", path: "/", contains: []string{"LLM Chef", `hx-get="/ingredients"`, `hx-post="/ask"`}},
		{name: "empty list", method: "GET", path: "/ingredients", contains: []string{`id="ingredients"`, `hx-post="/ingredients"`}},
		{name: "add", method: "POST", path: "/ingredients", form: url.Values{"name": {`<script>alert("x")</script>`}}, contains: []string{"&lt;script&gt;", "/ingredients/1"}, excludes: []string{"<script>"}, count: 1},
		{name: "empty name ignored", method: "POST", path: "/ingredients", form: url.Values{"name": {""}}, count: 1},
		{name: "missing name ignored", method: "POST", path: "/ingredients", count: 1},
		{name: "list saved ingredient", method: "GET", path: "/ingredients", contains: []string{"&lt;script&gt;"}, count: 1},
		{name: "delete", method: "DELETE", path: "/ingredients/1", excludes: []string{"&lt;script&gt;", `id="ingredient-1"`}},
		{name: "delete missing", method: "DELETE", path: "/ingredients/999"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := serveForm(handler, tc.method, tc.path, tc.form)
			if w.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			for _, fragment := range tc.contains {
				if !strings.Contains(w.Body.String(), fragment) {
					t.Errorf("body missing %q: %s", fragment, w.Body.String())
				}
			}
			for _, fragment := range tc.excludes {
				if strings.Contains(w.Body.String(), fragment) {
					t.Errorf("body contains %q: %s", fragment, w.Body.String())
				}
			}
			ings, err := repo.FindAll(context.Background())
			if err != nil || len(ings) != tc.count {
				t.Fatalf("ingredients=%v, err=%v; want count %d", ings, err, tc.count)
			}
		})
	}
}

func TestHTTPInvalidIngredientID(t *testing.T) {
	repo, _ := testRepository(t)
	seedIngredient(t, repo, "にんじん")
	handler := newHandler(repo, testClient(t, nil))
	for _, id := range []string{"abc", "1.5", "9999999999999999999999999999999999"} {
		t.Run(id, func(t *testing.T) {
			w := serveForm(handler, "DELETE", "/ingredients/"+id, nil)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status=%d, want 400", w.Code)
			}
		})
	}
	ings, err := repo.FindAll(context.Background())
	if err != nil || len(ings) != 1 {
		t.Fatalf("invalid deletion changed ingredients: %v, err=%v", ings, err)
	}
}

func TestHTTPDatabaseFailures(t *testing.T) {
	for _, tc := range []struct{ method, path string }{
		{"GET", "/"}, {"GET", "/ingredients"}, {"POST", "/ingredients"},
		{"DELETE", "/ingredients/1"}, {"POST", "/ask"},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			repo, db := testRepository(t)
			db.Close()
			w := serveForm(newHandler(repo, testClient(t, nil)), tc.method, tc.path, url.Values{"name": {"potato"}})
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status=%d, want 500; body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestHTTPWriteFailures(t *testing.T) {
	for _, tc := range []struct{ operation, method, path string }{
		{"INSERT", "POST", "/ingredients"}, {"DELETE", "DELETE", "/ingredients/1"},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			repo, db := testRepository(t)
			seedIngredient(t, repo, "potato")
			_, err := db.Exec(fmt.Sprintf(`CREATE TRIGGER reject_write BEFORE %s ON ingredients BEGIN SELECT RAISE(FAIL, 'write rejected'); END`, tc.operation))
			if err != nil {
				t.Fatal(err)
			}
			w := serveForm(newHandler(repo, testClient(t, nil)), tc.method, tc.path, url.Values{"name": {"onion"}})
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status=%d, want 500; body=%s", w.Code, w.Body.String())
			}
			ings, err := repo.FindAll(context.Background())
			if err != nil || len(ings) != 1 || ings[0].Name != "potato" {
				t.Fatalf("failed write changed ingredients: %v, err=%v", ings, err)
			}
		})
	}
}

func TestAsk(t *testing.T) {
	for method, label := range map[string]string{"microwave": "電子レンジ", "pan": "フライパン", "hotcook": "ホットクック"} {
		t.Run(method, func(t *testing.T) {
			repo, _ := testRepository(t)
			seedIngredient(t, repo, "にんじん")
			seedIngredient(t, repo, "玉ねぎ")
			calls := 0
			contextKey := struct{}{}
			client := testClient(t, func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Context().Value(contextKey) != "request context" {
					t.Error("API request did not inherit the HTTP request context")
				}
				if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
					t.Errorf("unexpected API endpoint: %s %s", r.Method, r.URL)
				}
				var body struct{ Model, Input string }
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body.Model != string(openai.ChatModelGPT5_4) {
					t.Errorf("model=%q", body.Model)
				}
				for _, text := range []string{label, "にんじん, 玉ねぎ", "3個程度レシピ"} {
					if !strings.Contains(body.Input, text) {
						t.Errorf("prompt missing %q: %s", text, body.Input)
					}
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"resp_test","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"## レシピ\n\n**にんじん炒め**\n\n<script>alert(1)</script>"}]}]}`))}, nil
			})
			req := httptest.NewRequest("POST", "/ask", strings.NewReader(url.Values{"method": {method}}.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req = req.WithContext(context.WithValue(req.Context(), contextKey, "request context"))
			w := httptest.NewRecorder()
			newHandler(repo, client).ServeHTTP(w, req)
			if w.Code != http.StatusOK || calls != 1 {
				t.Fatalf("status=%d, API calls=%d, body=%s", w.Code, calls, w.Body.String())
			}
			for _, fragment := range []string{`id="llm"`, "<h2>レシピ</h2>", "<strong>にんじん炒め</strong>"} {
				if !strings.Contains(w.Body.String(), fragment) {
					t.Errorf("body missing %q: %s", fragment, w.Body.String())
				}
			}
			if strings.Contains(w.Body.String(), "<script>") {
				t.Errorf("unsafe HTML rendered: %s", w.Body.String())
			}
		})
	}
}

func TestAskAPIFailures(t *testing.T) {
	for _, failure := range []string{"transport", "status", "invalid JSON"} {
		t.Run(failure, func(t *testing.T) {
			repo, _ := testRepository(t)
			client := testClient(t, func(r *http.Request) (*http.Response, error) {
				if failure == "transport" {
					return nil, errors.New("connection failed")
				}
				status, body := http.StatusOK, "invalid JSON"
				if failure == "status" {
					status, body = http.StatusTooManyRequests, `{"error":{"message":"rate limited"}}`
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			w := serveForm(newHandler(repo, client), "POST", "/ask", url.Values{"method": {"pan"}})
			if w.Code != http.StatusBadGateway {
				t.Fatalf("status=%d, want 502; body=%s", w.Code, w.Body.String())
			}
		})
	}
}
