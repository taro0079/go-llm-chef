package main

import (
	"context"
	"database/sql"
	"html/template"
	"log"
	"net/http"
	"strconv"

	_ "github.com/mattn/go-sqlite3"
)

type Ingredient struct {
	ID   int64
	Name string
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
<h1>Items</h1>
<div hx-get="/ingredients" hx-trigger="load" id="result"></div>
</body>
</html>
{{define "ingredients"}}
<li id="ingredient-{{.ID}}">
<input id="ingredient-{{.ID}}" value="{{.Name}}">

</li>
{{end}}
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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		ings, err := repo.FindAll(r.Context())
		if err != nil {
			log.Fatal(err)
		}
		_ = pageTmpl.Execute(w, ings)
	})
	mux.HandleFunc("GET /ingredients", func(w http.ResponseWriter, r *http.Request) {
		ings, err := repo.FindAll(r.Context())
		if err != nil {
			log.Fatal(err)
		}

		_ = ingTempl.Execute(w, ings)
	})
	mux.HandleFunc("POST /ingredients", func(w http.ResponseWriter, r *http.Request) {

		if r.FormValue("name") != "" {
			repo.Create(r.Context(), &Ingredient{Name: r.FormValue("name")})

		}
		ings, err := repo.FindAll(r.Context())
		if err != nil {
			log.Fatal(err)
		}
		_ = ingTempl.Execute(w, ings)
	})

	mux.HandleFunc("DELETE /ingredients/{ingredient_id}", func(w http.ResponseWriter, r *http.Request) {
			id, err := strconv.Atoi(r.PathValue("ingredient_id"))
		if err != nil {
			log.Fatal(err)
		}

		repo.Delete(r.Context(), id)
		ings, err := repo.FindAll(r.Context())
		if err != nil {
			log.Fatal(err)
		}
		_ = ingTempl.Execute(w, ings)
	})

	log.Println("listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
