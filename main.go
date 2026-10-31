package main

import (
	"html/template"
	"log"
	"net/http"
)

type Ingredient struct {
		ID int
		Name string
}

var ingredients = []Ingredient{
		{ID: 1, Name: "にんじん"},
		{ID: 1, Name: "トマト"},		
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
<button type="submit">Submit</button>
</form>
</li>
</ul>
{{define "ingredients"}}
<li id="ingredient-{{.ID}}">
<input id="ingredient-{{.ID}}" value="{{.Name}}">
</li>
{{end}}

`))

func main() {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
				_ = pageTmpl.Execute(w, ingredients)
		})
		mux.HandleFunc("GET /ingredients", func(w http.ResponseWriter, r *http.Request) {
				_ = ingTempl.Execute(w, ingredients)
		})
		mux.HandleFunc("POST /ingredients", func(w http.ResponseWriter, r *http.Request) {

				var ings []Ingredient
				if r.FormValue("name") != "" {
						ings = append(ingredients, Ingredient{Name: r.FormValue("name")})
				} else {
						ings = ingredients
				}

				_ = ingTempl.Execute(w, ings)
		})

		
		
		log.Println("listening on http://localhost:8080")
		log.Fatal(http.ListenAndServe(":8080", mux))
}
