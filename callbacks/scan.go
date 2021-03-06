package callbacks

import (
	"database/sql"
	"reflect"

	"github.com/jinzhu/gorm"
	"github.com/jinzhu/gorm/schema"
)

func Scan(rows *sql.Rows, db *gorm.DB) {
	columns, _ := rows.Columns()
	values := make([]interface{}, len(columns))

	switch dest := db.Statement.Dest.(type) {
	case map[string]interface{}, *map[string]interface{}:
		for idx, _ := range columns {
			values[idx] = new(interface{})
		}

		if rows.Next() {
			db.RowsAffected++
			rows.Scan(values...)
		}

		mapValue, ok := dest.(map[string]interface{})
		if ok {
			if v, ok := dest.(*map[string]interface{}); ok {
				mapValue = *v
			}
		}

		for idx, column := range columns {
			mapValue[column] = *(values[idx].(*interface{}))
		}
	case *[]map[string]interface{}:
		for idx, _ := range columns {
			values[idx] = new(interface{})
		}

		for rows.Next() {
			db.RowsAffected++
			rows.Scan(values...)

			v := map[string]interface{}{}
			for idx, column := range columns {
				v[column] = *(values[idx].(*interface{}))
			}
			*dest = append(*dest, v)
		}
	default:
		switch db.Statement.ReflectValue.Kind() {
		case reflect.Slice, reflect.Array:
			isPtr := db.Statement.ReflectValue.Type().Elem().Kind() == reflect.Ptr
			db.Statement.ReflectValue.Set(reflect.MakeSlice(db.Statement.ReflectValue.Type(), 0, 0))
			fields := make([]*schema.Field, len(columns))

			for idx, column := range columns {
				fields[idx] = db.Statement.Schema.LookUpField(column)
			}

			for rows.Next() {
				elem := reflect.New(db.Statement.Schema.ModelType).Elem()
				for idx, field := range fields {
					if field != nil {
						values[idx] = field.ReflectValueOf(elem).Addr().Interface()
					}
				}

				db.RowsAffected++
				if err := rows.Scan(values...); err != nil {
					db.AddError(err)
				}

				if isPtr {
					db.Statement.ReflectValue.Set(reflect.Append(db.Statement.ReflectValue, elem.Addr()))
				} else {
					db.Statement.ReflectValue.Set(reflect.Append(db.Statement.ReflectValue, elem))
				}
			}
		case reflect.Struct:
			for idx, column := range columns {
				if field := db.Statement.Schema.LookUpField(column); field != nil {
					values[idx] = field.ReflectValueOf(db.Statement.ReflectValue).Addr().Interface()
				} else {
					values[idx] = sql.RawBytes{}
				}
			}

			if rows.Next() {
				db.RowsAffected++
				if err := rows.Scan(values...); err != nil {
					db.AddError(err)
				}
			}
		}
	}

	if db.RowsAffected == 0 && db.Statement.RaiseErrorOnNotFound {
		db.AddError(gorm.ErrRecordNotFound)
	}
}
