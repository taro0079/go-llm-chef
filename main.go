package main

import (
	"bytes"
	"context"
	"database/sql"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/yuin/goldmark"
)

type Ingredient struct {
	ID   int64
	Name string
}

type Prompt struct {
	Ingredients string
	Method      string
}

var pageTmpl = template.Must(template.New("page").Parse(`
<!doctype html>
<html lang="ja">
<head>
  <meta charset="utf-8">
  <title>LLM Chef</title>
  <script src="https://unpkg.com/htmx.org@4.0/dist/htmx.min.js" defer></script>

</head>
<body>
<h1>My GPT Chef</h1>
<label for="result">材料</label>
<div hx-get="/ingredients" hx-trigger="load" id="result" hx-indicator="#loading" name="result"></div>

<form
  hx-post="/ask"
  hx-target="#llm-result"
  hx-swap="innerHTML"
  hx-indicator="#loading"

>
<div id="method">
<label for="method">調理方法</label>
<select id="method" name="method">
  <option value="hotcook">ホットクック</option>
  <option value="microwave">電子レンジ</option>
  <option value="pan">フライパン</option>
</select>
</div>
<button type="submit">ASK</button>
</form>
<div id="loading" class="spinner"></div>
<div id="llm-result">
</div>
</body>
<style>
.ask-form {
  display: flex;
  flex-direction: column;
  gap: 12px;
  align-items: center;
}
.spinner {
  display: none;
  width: 24px;
  height: 24px;
  border: 3px solid #ddd;
  border-top-color: #3498db;
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
.spinner.htmx-request {
  display: inline-block;
}
</style>
</html>
`))

var ingTempl = template.Must(template.New("page").Parse(`
<ul id="ingredients">
{{range .}}{{template "ingredients" .}}{{end}}
<li id="new-ingredient">
<form hx-post="/ingredients" hx-target="#result" hx-swap="innerHTML">
<input name="name">
<button type="submit">Add</button>
</form>
</li>
</ul>
{{define "ingredients"}}
<li id="ingredient-{{.ID}}">
<input id="ingredient-{{.ID}}" value="{{.Name}}">
<button type="button" hx-delete="/ingredients/{{.ID}}" hx-target="#result" hx-swap="innerHTML">x</button>
</li>
{{end}}

`))

var llmAnswerTempl = template.Must(template.New("llm").Parse(`
<div id="llm">
{{.}}
</div>
`))

type IngredientRepository struct {
	db *sql.DB
}

func NewIngredientRepository(db *sql.DB) *IngredientRepository {
	return &IngredientRepository{db: db}
}
func (r *IngredientRepository) FindAll(ctx context.Context) ([]Ingredient, error) {
	query := `SELECT * FROM ingredients`
	rows, err := r.db.QueryContext(
		ctx,
		query,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ingredients := make([]Ingredient, 0)

	for rows.Next() {
		var ing Ingredient
		err := rows.Scan(
			&ing.ID,
			&ing.Name,
		)

		if err != nil {
			return nil, err
		}

		ingredients = append(ingredients, ing)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ingredients, nil

}
func (r *IngredientRepository) Create(ctx context.Context, ingredient *Ingredient) (*Ingredient, error) {
	query := `
            INSERT INTO ingredients (name)
            VALUES (?)
         `

	result, err := r.db.ExecContext(
		ctx,
		query,
		ingredient.Name,
	)

	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	ingredient.ID = id

	return ingredient, nil
}
func (r *IngredientRepository) Delete(ctx context.Context, id int) error {
	query := `
            DELETE FROM ingredients WHERE id = ?
         `

	_, err := r.db.ExecContext(
		ctx,
		query,
		id,
	)

	if err != nil {
		return err
	}

	return nil
}

func main() {
	err := godotenv.Load()
	if err != nil && !os.IsNotExist(err) {
		log.Fatal(err)
	}

	db, err := sql.Open("sqlite3", "./app.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	sqlTable := `
            CREATE TABLE IF NOT EXISTS ingredients (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                name TEXT
            )
        `
	_, err = db.Exec(sqlTable)
	if err != nil {
		log.Fatal(err)
	}
	repo := NewIngredientRepository(db)

	client := openai.NewClient(
		option.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
	)

	log.Println("listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", newHandler(repo, client)))
}

func newHandler(repo *IngredientRepository, client openai.Client) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		ings, err := repo.FindAll(r.Context())
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		_ = pageTmpl.Execute(w, ings)
	})
	mux.HandleFunc("GET /ingredients", func(w http.ResponseWriter, r *http.Request) {
		ings, err := repo.FindAll(r.Context())
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		_ = ingTempl.Execute(w, ings)
	})
	mux.HandleFunc("POST /ingredients", func(w http.ResponseWriter, r *http.Request) {

		if r.FormValue("name") != "" {
			if _, err := repo.Create(r.Context(), &Ingredient{Name: r.FormValue("name")}); err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}

		}
		ings, err := repo.FindAll(r.Context())
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		_ = ingTempl.Execute(w, ings)
	})

	mux.HandleFunc("DELETE /ingredients/{ingredient_id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("ingredient_id"))
		if err != nil {
			http.Error(w, "invalid ingredient id", http.StatusBadRequest)
			return
		}

		if err := repo.Delete(r.Context(), id); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		ings, err := repo.FindAll(r.Context())
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		_ = ingTempl.Execute(w, ings)
	})

	mux.HandleFunc("POST /ask", func(w http.ResponseWriter, r *http.Request) {
		methodMap := map[string]string{
			"microwave": "電子レンジ",
			"pan":       "フライパン",
			"hotcook":   "ホットクック",
		}
		cookMethod := methodMap[r.FormValue("method")]

		ings, err := repo.FindAll(r.Context())
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		var names []string

		for _, ing := range ings {
			names = append(names, ing.Name)
		}
		joined := strings.Join(names, ", ")

		ctx := r.Context()
		question := `
あなたは料理が上手な主婦です。調理方法と材料を指定するので3個程度レシピを提案してください。
## 調理方法
{{ .Method }}

## 材料
{{ .Ingredients }}
`
		tmpl, err := template.New("prompt").Parse(question)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		data := Prompt{
			Ingredients: joined,
			Method:      cookMethod,
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		finalQuestion := buf.String()
		res, err := client.Responses.New(ctx, responses.ResponseNewParams{
			Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(finalQuestion)},
			Model: openai.ChatModelGPT5_4,
		})

		if err != nil {
			http.Error(w, "recipe service unavailable", http.StatusBadGateway)
			return
		}
		source := []byte(res.OutputText())
		var llmBuf bytes.Buffer
		if err := goldmark.Convert(source, &llmBuf); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		_ = llmAnswerTempl.Execute(w, template.HTML(llmBuf.String()))

	})

	return mux
}
