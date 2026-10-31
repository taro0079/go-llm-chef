package main

import (
	"bytes"
	"database/sql"
	"embed"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/taro0079/go-llm-chef/internal/repository"

	"github.com/joho/godotenv"
	_ "github.com/mattn/go-sqlite3"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/yuin/goldmark"
)

type Prompt struct {
	Ingredients string
	Method      string
}

//go:embed templates/*
var templateFiles embed.FS

var pageTmpl = template.Must(template.ParseFS(templateFiles, "templates/page.html"))

var ingTempl = template.Must(template.ParseFS(templateFiles, "templates/ingredients.html"))

var llmAnswerTempl = template.Must(template.ParseFS(templateFiles, "templates/llm_answer.html"))

var promptTmpl = template.Must(template.ParseFS(templateFiles, "templates/prompt.tmpl"))

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
	repo := repository.NewIngredientRepository(db)

	client := openai.NewClient(
		option.WithAPIKey(os.Getenv("OPENAI_API_KEY")),
	)

	log.Println("listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", newHandler(repo, client)))
}

func newHandler(repo *repository.IngredientRepository, client openai.Client) http.Handler {
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
			if _, err := repo.Create(r.Context(), &repository.Ingredient{Name: r.FormValue("name")}); err != nil {
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

		data := Prompt{
			Ingredients: joined,
			Method:      cookMethod,
		}

		var buf bytes.Buffer
		if err := promptTmpl.Execute(&buf, data); err != nil {
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
