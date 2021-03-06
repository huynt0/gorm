package gorm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/jinzhu/gorm/clause"
	"github.com/jinzhu/gorm/schema"
)

// Statement statement
type Statement struct {
	*DB
	Table                string
	Model                interface{}
	Dest                 interface{}
	ReflectValue         reflect.Value
	Clauses              map[string]clause.Clause
	Selects              []string // selected columns
	Omits                []string // omit columns
	Settings             sync.Map
	ConnPool             ConnPool
	Schema               *schema.Schema
	Context              context.Context
	RaiseErrorOnNotFound bool
	SQL                  strings.Builder
	Vars                 []interface{}
	NamedVars            []sql.NamedArg
}

// StatementModifier statement modifier interface
type StatementModifier interface {
	ModifyStatement(*Statement)
}

// Write write string
func (stmt *Statement) WriteString(str string) (int, error) {
	return stmt.SQL.WriteString(str)
}

// Write write string
func (stmt *Statement) WriteByte(c byte) error {
	return stmt.SQL.WriteByte(c)
}

// WriteQuoted write quoted value
func (stmt *Statement) WriteQuoted(value interface{}) error {
	stmt.QuoteTo(&stmt.SQL, value)
	return nil
}

// QuoteTo write quoted value to writer
func (stmt Statement) QuoteTo(writer clause.Writer, field interface{}) {
	switch v := field.(type) {
	case clause.Table:
		if v.Name == clause.CurrentTable {
			stmt.DB.Dialector.QuoteTo(writer, stmt.Table)
		} else {
			stmt.DB.Dialector.QuoteTo(writer, v.Name)
		}

		if v.Alias != "" {
			writer.WriteString(" AS ")
			stmt.DB.Dialector.QuoteTo(writer, v.Alias)
		}
	case clause.Column:
		if v.Table != "" {
			if v.Table == clause.CurrentTable {
				stmt.DB.Dialector.QuoteTo(writer, stmt.Table)
			} else {
				stmt.DB.Dialector.QuoteTo(writer, v.Table)
			}
			writer.WriteByte('.')
		}

		if v.Name == clause.PrimaryKey {
			if stmt.Schema != nil && stmt.Schema.PrioritizedPrimaryField != nil {
				stmt.DB.Dialector.QuoteTo(writer, stmt.Schema.PrioritizedPrimaryField.DBName)
			}
		} else {
			stmt.DB.Dialector.QuoteTo(writer, v.Name)
		}

		if v.Alias != "" {
			writer.WriteString(" AS ")
			stmt.DB.Dialector.QuoteTo(writer, v.Alias)
		}
	case string:
		stmt.DB.Dialector.QuoteTo(writer, v)
	default:
		stmt.DB.Dialector.QuoteTo(writer, fmt.Sprint(field))
	}
}

// Quote returns quoted value
func (stmt Statement) Quote(field interface{}) string {
	var builder strings.Builder
	stmt.QuoteTo(&builder, field)
	return builder.String()
}

// Write write string
func (stmt *Statement) AddVar(writer clause.Writer, vars ...interface{}) {
	for idx, v := range vars {
		if idx > 0 {
			writer.WriteByte(',')
		}

		switch v := v.(type) {
		case sql.NamedArg:
			if len(v.Name) > 0 {
				stmt.NamedVars = append(stmt.NamedVars, v)
				writer.WriteByte('@')
				writer.WriteString(v.Name)
			} else {
				stmt.Vars = append(stmt.Vars, v.Value)
				stmt.DB.Dialector.BindVarTo(writer, stmt, v.Value)
			}
		case clause.Column, clause.Table:
			stmt.QuoteTo(writer, v)
		case clause.Expr:
			writer.WriteString(v.SQL)
			stmt.Vars = append(stmt.Vars, v.Vars...)
		case []interface{}:
			if len(v) > 0 {
				writer.WriteByte('(')
				stmt.AddVar(writer, v...)
				writer.WriteByte(')')
			} else {
				writer.WriteString("(NULL)")
			}
		default:
			stmt.Vars = append(stmt.Vars, v)
			stmt.DB.Dialector.BindVarTo(writer, stmt, v)
		}
	}
}

// AddClause add clause
func (stmt *Statement) AddClause(v clause.Interface) {
	if optimizer, ok := v.(StatementModifier); ok {
		optimizer.ModifyStatement(stmt)
	}

	c, ok := stmt.Clauses[v.Name()]
	if !ok {
		c.Name = v.Name()
	}
	v.MergeClause(&c)
	stmt.Clauses[v.Name()] = c
}

// AddClauseIfNotExists add clause if not exists
func (stmt *Statement) AddClauseIfNotExists(v clause.Interface) {
	if _, ok := stmt.Clauses[v.Name()]; !ok {
		stmt.AddClause(v)
	}
}

// BuildCondtion build condition
func (stmt Statement) BuildCondtion(query interface{}, args ...interface{}) (conditions []clause.Expression) {
	if sql, ok := query.(string); ok {
		if i, err := strconv.Atoi(sql); err == nil {
			query = i
		} else if len(args) == 0 || (len(args) > 0 && strings.Contains(sql, "?")) || strings.Contains(sql, "@") {
			return []clause.Expression{clause.Expr{SQL: sql, Vars: args}}
		}
	}

	args = append([]interface{}{query}, args...)
	for _, arg := range args {
		if valuer, ok := arg.(driver.Valuer); ok {
			arg, _ = valuer.Value()
		}

		switch v := arg.(type) {
		case clause.Expression:
			conditions = append(conditions, v)
		case *DB:
			if v.Statement == nil {
				if cs, ok := v.Statement.Clauses["WHERE"]; ok {
					conditions = append(conditions, cs.Expression)
				}
			}
		case map[interface{}]interface{}:
			var clauseMap = clause.Map{}
			for i, j := range v {
				clauseMap[i] = j
			}
			conditions = append(conditions, clauseMap)
		case map[string]string:
			var clauseMap = clause.Map{}
			for i, j := range v {
				clauseMap[i] = j
			}
			conditions = append(conditions, clauseMap)
		case map[string]interface{}:
			var clauseMap = clause.Map{}
			for i, j := range v {
				clauseMap[i] = j
			}
			conditions = append(conditions, clauseMap)
		default:
			// TODO check is struct
			// struct, slice -> ids
		}
	}

	if len(conditions) == 0 {
		conditions = append(conditions, clause.IN{Column: clause.PrimaryColumn, Values: args})
	}

	return conditions
}

// Build build sql with clauses names
func (stmt *Statement) Build(clauses ...string) {
	var firstClauseWritten bool

	for _, name := range clauses {
		if c, ok := stmt.Clauses[name]; ok {
			if firstClauseWritten {
				stmt.WriteByte(' ')
			}

			firstClauseWritten = true
			if b, ok := stmt.DB.ClauseBuilders[name]; ok {
				b.Build(c, stmt)
			} else {
				c.Build(stmt)
			}
		}
	}
	// TODO handle named vars
}

func (stmt *Statement) Parse(value interface{}) (err error) {
	if stmt.Schema, err = schema.Parse(value, stmt.DB.cacheStore, stmt.DB.NamingStrategy); err == nil && stmt.Table == "" {
		stmt.Table = stmt.Schema.Table
	}
	return err
}

func (stmt *Statement) reinit() {
	stmt.Table = ""
	stmt.Model = nil
	stmt.Selects = nil
	stmt.Omits = nil
	stmt.ConnPool = stmt.DB.Config.ConnPool
	stmt.Schema = nil
	stmt.Context = context.Background()
	stmt.RaiseErrorOnNotFound = false

	stmt.SQL.Reset()
	stmt.Vars = nil
	stmt.NamedVars = nil

	for k := range stmt.Clauses {
		delete(stmt.Clauses, k)
	}

	stmt.Settings.Range(func(k, _ interface{}) bool {
		stmt.Settings.Delete(k)
		return true
	})
}
