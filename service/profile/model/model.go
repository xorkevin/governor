package profilemodel

import (
	"database/sql"
	"fmt"
	"github.com/hackform/governor"
	"net/http"
)

const (
	tableName     = "profiles"
	moduleID      = "profilemodel"
	moduleIDModel = moduleID + ".Model"
)

type (
	// Model is the db profile model
	Model struct {
	}
)

const (
	moduleIDSetup = moduleID + ".Setup"
)

var (
	sqlSetup = fmt.Sprintf("CREATE TABLE %s (userid BYTEA PRIMARY KEY);", tableName)
)

// Setup creates a new Profile table
func Setup(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlSetup)
	if err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
