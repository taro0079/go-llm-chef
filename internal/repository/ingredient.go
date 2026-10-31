package repository

import (
	"context"
	"database/sql"
)

type Ingredient struct {
	ID   int64
	Name string
}

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
