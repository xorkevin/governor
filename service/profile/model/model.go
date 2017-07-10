package profilemodel

import (
	"database/sql"
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/lib/pq"
	"net/http"
	"sort"
	"strings"
)

const (
	tableName     = "profiles"
	moduleID      = "profilemodel"
	moduleIDModel = moduleID + ".Model"
)

type (
	// Model is the db profile model
	Model struct {
		Userid       []byte `json:"userid"`
		Email        string `json:"contact_email"`
		Bio          string `json:"bio"`
		Image        string `json:"profile_image_url"`
		PublicFields string `json:"public_fields"`
	}
)

const (
	moduleIDModSetIDB64 = moduleIDModel + ".SetIDB64"
)

// SetIDB64 sets the userid of the model from a base64 value
func (m *Model) SetIDB64(idb64 string) *governor.Error {
	u, err := usermodel.ParseB64ToUID(idb64)
	if err != nil {
		err.AddTrace(moduleIDModSetIDB64)
		return err
	}
	m.Userid = u.Bytes()
	return nil
}

const (
	moduleIDModSetPublic = moduleIDModel + ".SetPublic"
)

// SetPublic sets the public fields of the model
func (m *Model) SetPublic(add []string, remove []string) *governor.Error {
	s := strings.Split(m.PublicFields, ",")
	s = append(s, add...)
	sort.Strings(s)
	sort.Strings(remove)
	i := 0
	j := 0
	for i < len(s) && j < len(remove) {
		if s[i] == remove[j] {
			s = append(s[:i], s[i+1:]...)
			j++
		} else {
			i++
		}
	}
	m.PublicFields = strings.Join(s, ",")
	return nil
}

const (
	moduleIDModB64 = moduleIDModel + ".IDBase64"
)

// IDBase64 returns the userid as a base64 encoded string
func (m *Model) IDBase64() (string, *governor.Error) {
	u, err := usermodel.ParseUIDToB64(m.Userid)
	if err != nil {
		err.AddTrace(moduleIDModB64)
		return "", err
	}
	return u.Base64(), nil
}

const (
	moduleIDModGet64 = moduleIDModel + ".GetByIDB64"
)

var (
	sqlGetByIDB64 = fmt.Sprintf("SELECT userid, contact_email, bio, profile_image_url, public_fields FROM %s WHERE userid=$1;", tableName)
)

// GetByIDB64 returns a profile model with the given base64 id
func GetByIDB64(db *sql.DB, idb64 string) (*Model, *governor.Error) {
	u, err := usermodel.ParseB64ToUID(idb64)
	if err != nil {
		err.AddTrace(moduleIDModGet64)
		return nil, err
	}
	mUser := &Model{}
	if err := db.QueryRow(sqlGetByIDB64, u.Bytes()).Scan(&mUser.Userid, &mUser.Email, &mUser.Bio, &mUser.Image, &mUser.PublicFields); err != nil {
		if err == sql.ErrNoRows {
			return nil, governor.NewError(moduleIDModGet64, "no user found with that id", 2, http.StatusNotFound)
		}
		return nil, governor.NewError(moduleIDModGet64, err.Error(), 0, http.StatusInternalServerError)
	}
	return mUser, nil
}

const (
	moduleIDModIns = moduleIDModel + ".Insert"
)

var (
	sqlInsert = fmt.Sprintf("INSERT INTO %s (userid, contact_email, bio, profile_image_url, public_fields) VALUES ($1, $2, $3, $4, $5);", tableName)
)

// Insert inserts the model into the db
func (m *Model) Insert(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlInsert, m.Userid, m.Email, m.Bio, m.Image, m.PublicFields)
	if err != nil {
		if postgresErr, ok := err.(*pq.Error); ok {
			switch postgresErr.Code {
			case "23505": // unique_violation
				return governor.NewErrorUser(moduleIDModIns, err.Error(), 3, http.StatusBadRequest)
			default:
				return governor.NewError(moduleIDModIns, err.Error(), 0, http.StatusInternalServerError)
			}
		}
	}
	return nil
}

const (
	moduleIDModUp = moduleIDModel + ".Update"
)

var (
	sqlUpdate = fmt.Sprintf("UPDATE %s SET (userid, contact_email, bio, profile_image_url, public_fields) = ($1, $2, $3, $4, $5) WHERE userid = $1;", tableName)
)

// Update updates the model in the db
func (m *Model) Update(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlUpdate, m.Userid, m.Email, m.Bio, m.Image, m.PublicFields)
	if err != nil {
		return governor.NewError(moduleIDModUp, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDModDel = moduleIDModel + ".Delete"
)

var (
	sqlDelete = fmt.Sprintf("DELETE FROM %s WHERE userid = $1;", tableName)
)

// Delete deletes the model in the db
func (m *Model) Delete(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlDelete, m.Userid)
	if err != nil {
		return governor.NewError(moduleIDModDel, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDSetup = moduleID + ".Setup"
)

var (
	sqlSetup = fmt.Sprintf("CREATE TABLE %s (userid BYTEA PRIMARY KEY, contact_email VARCHAR(4096), bio VARCHAR(4096), profile_image_url VARCHAR(4096), public_fields VARCHAR(4096));", tableName)
)

// Setup creates a new Profile table
func Setup(db *sql.DB) *governor.Error {
	_, err := db.Exec(sqlSetup)
	if err != nil {
		return governor.NewError(moduleIDSetup, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
