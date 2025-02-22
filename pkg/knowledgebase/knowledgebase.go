package knowledgebase

import (
	"context"

	"github.com/doubletabai/doubletab/pkg/vector"
)

const (
	sampleOtherDB = `Other databases support.
Currently only PostgreSQL DB is supported. We'll be adding support for other databases in the future. You're welcome
to suggest a database you'd like to see supported by DoubleTab by starting a discussion on GitHub:
https://github.com/doubletabai/doubletab/discussions/categories/ideas.
`
	sampleServerGo = `Example of a server implementation in Go based on OpenAPI 3.0 spec.

package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

type Server struct {
	DB *sqlx.DB
}

func (s Server) ListResources(w http.ResponseWriter, r *http.Request) {
	resources := []Resource{}
	err := s.DB.SelectContext(r.Context(), &resources, "SELECT * FROM resources")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err = json.NewEncoder(w).Encode(resources); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s Server) GetResource(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	resource := Resource{}
	err := s.DB.GetContext(r.Context(), &resource, "SELECT * FROM resources WHERE id = $1", id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err = json.NewEncoder(w).Encode(resource); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s Server) CreateResource(w http.ResponseWriter, r *http.Request) {
	var resource Resource
	if err := json.NewDecoder(r.Body).Decode(&resource); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resource.Id = uuid.New()

	_, err := s.DB.NamedExecContext(r.Context(), "INSERT INTO resources (id, name, email) VALUES (:id, name, :email)", &resource)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (s Server) UpdateResource(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	var resource Resource
	if err := json.NewDecoder(r.Body).Decode(&resource); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resource.Id = id

	res, err := s.DB.NamedExecContext(r.Context(), "UPDATE resources SET name = :name, email = :email WHERE id = :id", resource)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	n, err := res.RowsAffected()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if n == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s Server) DeleteResource(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	_, err := s.DB.ExecContext(r.Context(), "DELETE FROM resources WHERE id = $1", id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}`
)

func Populate(ctx context.Context, db *vector.KnowledgeService) error {
	if err := db.Store(ctx, sampleOtherDB); err != nil {
		return err
	}

	if err := db.Store(ctx, sampleServerGo); err != nil {
		return err
	}

	return nil
}
