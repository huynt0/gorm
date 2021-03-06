package callbacks

import (
	"github.com/jinzhu/gorm"
)

func RawExec(db *gorm.DB) {
	result, err := db.Statement.ConnPool.ExecContext(db.Statement.Context, db.Statement.SQL.String(), db.Statement.Vars...)
	if err != nil {
		db.AddError(err)
	} else {
		db.RowsAffected, _ = result.RowsAffected()
	}
}
