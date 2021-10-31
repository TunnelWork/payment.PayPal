package sqlwrapper

import (
	"database/sql"
	"errors"
	"strings"
)

// InitializeTables() is not responsible to close the input *sql.DB
func InitializeTables(db *sql.DB, tbl string) error {
	if db == nil {
		return errors.New("sqlhelper: nil db, no installation could be done")
	}

	var ordersTblCreationQuery string = ordersTblCreation
	ordersTblCreationQuery = strings.Replace(ordersTblCreationQuery, "paypal_orders", tbl, -1)

	// orders
	stmtOrdersTblCreation, err := db.Prepare(ordersTblCreationQuery)
	if err != nil {
		return err
	}
	defer stmtOrdersTblCreation.Close()

	_, err = stmtOrdersTblCreation.Exec()
	if err != nil {
		return nil
	}

	return nil
}
