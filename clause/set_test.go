package clause_test

import (
	"fmt"
	"testing"

	"github.com/jinzhu/gorm/clause"
)

func TestSet(t *testing.T) {
	results := []struct {
		Clauses []clause.Interface
		Result  string
		Vars    []interface{}
	}{
		{
			[]clause.Interface{
				clause.Update{},
				clause.Set([]clause.Assignment{{clause.PrimaryColumn, 1}}),
			},
			"UPDATE `users` SET `users`.`id`=?", []interface{}{1},
		},
		{
			[]clause.Interface{
				clause.Update{},
				clause.Set([]clause.Assignment{{clause.PrimaryColumn, 1}}),
				clause.Set([]clause.Assignment{{clause.Column{Name: "name"}, "jinzhu"}}),
			},
			"UPDATE `users` SET `users`.`id`=?,`name`=?", []interface{}{1, "jinzhu"},
		},
	}

	for idx, result := range results {
		t.Run(fmt.Sprintf("case #%v", idx), func(t *testing.T) {
			checkBuildClauses(t, result.Clauses, result.Result, result.Vars)
		})
	}
}
