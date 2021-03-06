package callbacks

import (
	"reflect"
	"sort"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/jinzhu/gorm/clause"
)

func BeforeUpdate(db *gorm.DB) {
	if db.Statement.Schema != nil && (db.Statement.Schema.BeforeSave || db.Statement.Schema.BeforeUpdate) {
		callMethod := func(value interface{}) bool {
			var ok bool
			if db.Statement.Schema.BeforeSave {
				if i, ok := value.(gorm.BeforeSaveInterface); ok {
					ok = true
					i.BeforeSave(db)
				}
			}

			if db.Statement.Schema.BeforeUpdate {
				if i, ok := value.(gorm.BeforeUpdateInterface); ok {
					ok = true
					i.BeforeUpdate(db)
				}
			}
			return ok
		}

		if ok := callMethod(db.Statement.Dest); !ok {
			switch db.Statement.ReflectValue.Kind() {
			case reflect.Slice, reflect.Array:
				for i := 0; i <= db.Statement.ReflectValue.Len(); i++ {
					callMethod(db.Statement.ReflectValue.Index(i).Interface())
				}
			case reflect.Struct:
				callMethod(db.Statement.ReflectValue.Interface())
			}
		}
	}
}

func Update(db *gorm.DB) {
	db.Statement.AddClauseIfNotExists(clause.Update{})
	db.Statement.AddClause(ConvertToAssignments(db.Statement))
	db.Statement.Build("UPDATE", "SET", "WHERE")

	result, err := db.Statement.ConnPool.ExecContext(db.Statement.Context, db.Statement.SQL.String(), db.Statement.Vars...)

	if err == nil {
		db.RowsAffected, _ = result.RowsAffected()
	} else {
		db.AddError(err)
	}
}

func AfterUpdate(db *gorm.DB) {
	if db.Statement.Schema != nil && (db.Statement.Schema.AfterSave || db.Statement.Schema.AfterUpdate) {
		callMethod := func(value interface{}) bool {
			var ok bool
			if db.Statement.Schema.AfterSave {
				if i, ok := value.(gorm.AfterSaveInterface); ok {
					ok = true
					i.AfterSave(db)
				}
			}

			if db.Statement.Schema.AfterUpdate {
				if i, ok := value.(gorm.AfterUpdateInterface); ok {
					ok = true
					i.AfterUpdate(db)
				}
			}
			return ok
		}

		if ok := callMethod(db.Statement.Dest); !ok {
			switch db.Statement.ReflectValue.Kind() {
			case reflect.Slice, reflect.Array:
				for i := 0; i <= db.Statement.ReflectValue.Len(); i++ {
					callMethod(db.Statement.ReflectValue.Index(i).Interface())
				}
			case reflect.Struct:
				callMethod(db.Statement.ReflectValue.Interface())
			}
		}
	}
}

// ConvertToAssignments convert to update assignments
func ConvertToAssignments(stmt *gorm.Statement) (set clause.Set) {
	selectColumns, restricted := SelectAndOmitColumns(stmt)
	reflectModelValue := reflect.ValueOf(stmt.Model)

	switch value := stmt.Dest.(type) {
	case map[string]interface{}:
		set = make([]clause.Assignment, 0, len(value))

		var keys []string
		for k, _ := range value {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			if field := stmt.Schema.LookUpField(k); field != nil {
				if v, ok := selectColumns[field.DBName]; (ok && v) || (!ok && !restricted) {
					if field.AutoUpdateTime > 0 {
						value[k] = time.Now()
					}
					set = append(set, clause.Assignment{Column: clause.Column{Name: field.DBName}, Value: value[k]})
					field.Set(reflectModelValue, value[k])
				}
			} else if v, ok := selectColumns[k]; (ok && v) || (!ok && !restricted) {
				set = append(set, clause.Assignment{Column: clause.Column{Name: k}, Value: value[k]})
			}
		}

		for _, field := range stmt.Schema.FieldsByDBName {
			if field.AutoUpdateTime > 0 && value[field.Name] == nil && value[field.DBName] == nil {
				now := time.Now()
				set = append(set, clause.Assignment{Column: clause.Column{Name: field.DBName}, Value: now})
				field.Set(reflectModelValue, now)
			}
		}
	default:
		switch stmt.ReflectValue.Kind() {
		case reflect.Struct:
			set = make([]clause.Assignment, 0, len(stmt.Schema.FieldsByDBName))
			for _, field := range stmt.Schema.FieldsByDBName {
				if !field.PrimaryKey || stmt.Dest != stmt.Model {
					if v, ok := selectColumns[field.DBName]; (ok && v) || (!ok && !restricted) {
						value, isZero := field.ValueOf(stmt.ReflectValue)
						if field.AutoUpdateTime > 0 {
							value = time.Now()
							isZero = false
						}

						if ok || !isZero {
							set = append(set, clause.Assignment{Column: clause.Column{Name: field.DBName}, Value: value})
							field.Set(reflectModelValue, value)
						}
					}
				} else {
					if value, isZero := field.ValueOf(stmt.ReflectValue); !isZero {
						stmt.AddClause(clause.Where{Exprs: []clause.Expression{clause.Eq{Column: field.DBName, Value: value}}})
					}
				}
			}
		}
	}

	if stmt.Dest != stmt.Model {
		reflectValue := reflect.ValueOf(stmt.Model)
		for _, field := range stmt.Schema.PrimaryFields {
			if value, isZero := field.ValueOf(reflectValue); !isZero {
				stmt.AddClause(clause.Where{Exprs: []clause.Expression{clause.Eq{Column: field.DBName, Value: value}}})
			}
		}
	}
	return
}
