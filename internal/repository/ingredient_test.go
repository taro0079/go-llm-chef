package repository

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Each test gets its own database; app.db and .env are never used.
func testRepository(t *testing.T) *IngredientRepository {
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
	return NewIngredientRepository(db)
}

func seedIngredient(t *testing.T, repo *IngredientRepository, name string) *Ingredient {
	t.Helper()
	ing, err := repo.Create(context.Background(), &Ingredient{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	return ing
}

func TestIngredientRepositoryCRUD(t *testing.T) {
	repo := testRepository(t)
	ctx := context.Background()
	ings, err := repo.FindAll(ctx)
	if err != nil || len(ings) != 0 {
		t.Fatalf("empty database: ingredients=%v, err=%v", ings, err)
	}

	first := seedIngredient(t, repo, "にんじん")
	second := seedIngredient(t, repo, "玉ねぎ ' < & >")
	if first.ID <= 0 || second.ID <= first.ID {
		t.Fatalf("expected distinct generated IDs, got %d and %d", first.ID, second.ID)
	}
	ings, err = repo.FindAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// FindAll has no ordering contract.
	got := make(map[int64]string)
	for _, ing := range ings {
		got[ing.ID] = ing.Name
	}
	want := map[int64]string{first.ID: first.Name, second.ID: second.Name}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ingredients=%v, want %v", got, want)
	}

	if err := repo.Delete(ctx, int(first.ID)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, int(first.ID)); err != nil {
		t.Fatalf("deleting missing ingredient: %v", err)
	}
	ings, err = repo.FindAll(ctx)
	if err != nil || !reflect.DeepEqual(ings, []Ingredient{*second}) {
		t.Fatalf("after delete: ingredients=%v, err=%v", ings, err)
	}
}

func TestIngredientRepositoryErrors(t *testing.T) {
	for _, failure := range []string{"closed database", "missing table", "canceled context"} {
		t.Run(failure, func(t *testing.T) {
			repo := testRepository(t)
			ctx := context.Background()
			switch failure {
			case "closed database":
				if err := repo.db.Close(); err != nil {
					t.Fatal(err)
				}
			case "missing table":
				if _, err := repo.db.Exec("DROP TABLE ingredients"); err != nil {
					t.Fatal(err)
				}
			case "canceled context":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			_, findErr := repo.FindAll(ctx)
			_, createErr := repo.Create(ctx, &Ingredient{Name: "potato"})
			deleteErr := repo.Delete(ctx, 1)
			for operation, err := range map[string]error{"find": findErr, "create": createErr, "delete": deleteErr} {
				if err == nil {
					t.Errorf("%s: expected error", operation)
				} else if failure == "canceled context" && !errors.Is(err, context.Canceled) {
					t.Errorf("%s: got %v, want context.Canceled", operation, err)
				}
			}
		})
	}
}

func TestIngredientRepositoryScanError(t *testing.T) {
	repo := testRepository(t)
	if _, err := repo.db.Exec("INSERT INTO ingredients (name) VALUES (NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindAll(context.Background()); err == nil {
		t.Fatal("expected error for NULL name")
	}
	// A failed scan must release the sole database connection.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := repo.db.PingContext(ctx); err != nil {
		t.Fatalf("connection not released after scan error: %v", err)
	}
}
