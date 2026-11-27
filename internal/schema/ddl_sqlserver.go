package schema

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lichenghsu/synccraft/internal/dialect"
)

type sqlserverDDL struct{ d dialect.SQLServer }

func (g sqlserverDDL) statements(cur *Table, c Change) ([]string, error) {
	d := g.d
	t := qualified(d, c.Table)

	switch c.Kind {
	case AddTable:
		return createTable(d, t, c.Create, g.columnDef, func(ix dialect.Index) string { return createIndexSQL(d, c.Table, ix) })
	case DropTable:
		return []string{"DROP TABLE " + t}, nil
	case AddColumn:
		return []string{"ALTER TABLE " + t + " ADD " + g.columnDef(*c.Column)}, nil
	case DropColumn:
		// A default constraint blocks DROP COLUMN.
		return []string{g.dropDefault(t, c.Column.Name), "ALTER TABLE " + t + " DROP COLUMN " + d.QuoteIdent(c.Column.Name)}, nil
	case AlterColumn:
		return g.alterColumn(t, *c.From, *c.Column)
	case AlterPrimaryKey:
		var out []string
		if len(c.FromPrimaryKey) > 0 {
			out = append(out, g.dropConstraintBy(t, "SELECT name FROM sys.key_constraints WHERE type = 'PK' AND parent_object_id = OBJECT_ID("+nLiteral(t)+")"))
		}
		if len(c.PrimaryKey) > 0 {
			out = append(out, "ALTER TABLE "+t+" ADD PRIMARY KEY ("+quoteList(d, c.PrimaryKey)+")")
		}
		return out, nil
	case AddIndex:
		if err := checkIndexColumns(*c.Index); err != nil {
			return nil, err
		}
		if c.Index.Constraint {
			return []string{"ALTER TABLE " + t + " ADD CONSTRAINT " + d.QuoteIdent(c.Index.Name) + " UNIQUE (" + quoteList(d, c.Index.Columns) + ")"}, nil
		}
		return []string{createIndexSQL(d, c.Table, *c.Index)}, nil
	case DropIndex:
		if c.Index.Constraint {
			return []string{"ALTER TABLE " + t + " DROP CONSTRAINT " + d.QuoteIdent(c.Index.Name)}, nil
		}
		return []string{"DROP INDEX " + d.QuoteIdent(c.Index.Name) + " ON " + t}, nil
	}
	return nil, fmt.Errorf("unknown change %q", c.Kind)
}

func (g sqlserverDDL) columnDef(c dialect.Column) string {
	s := g.d.QuoteIdent(c.Name) + " " + c.Type
	if c.AutoIncrement {
		s += " IDENTITY(1,1)"
	}
	s += nullClause(c)
	if c.Default != nil {
		s += " DEFAULT " + *c.Default
	}
	return s
}

// alterColumn drops and re-adds the default around ALTER COLUMN, because a
// default constraint blocks changing the column.
func (g sqlserverDDL) alterColumn(t string, from, to dialect.Column) ([]string, error) {
	if from.AutoIncrement != to.AutoIncrement {
		return nil, errors.New("SQL Server cannot add or remove IDENTITY on an existing column; recreate the table")
	}
	col := g.d.QuoteIdent(to.Name)
	alter := typeChanged(from, to) || from.Nullable != to.Nullable
	dflt := !defaultsEqual(from.Default, to.Default)

	var out []string
	if from.Default != nil && (alter || dflt) {
		out = append(out, g.dropDefault(t, to.Name))
	}
	if alter {
		null := " NULL"
		if !to.Nullable {
			null = " NOT NULL"
		}
		out = append(out, "ALTER TABLE "+t+" ALTER COLUMN "+col+" "+to.Type+null)
	}
	if to.Default != nil && (alter && from.Default != nil || dflt) {
		out = append(out, "ALTER TABLE "+t+" ADD DEFAULT "+*to.Default+" FOR "+col)
	}
	return out, nil
}

// dropDefault drops the column's default constraint, whose generated name
// is looked up on the server.
func (g sqlserverDDL) dropDefault(t, column string) string {
	return g.dropConstraintBy(t, "SELECT dc.name FROM sys.default_constraints dc"+
		" JOIN sys.columns c ON c.object_id = dc.parent_object_id AND c.column_id = dc.parent_column_id"+
		" WHERE dc.parent_object_id = OBJECT_ID("+nLiteral(t)+") AND c.name = "+nLiteral(column))
}

// dropConstraintBy drops the constraint named by query's single result, if any.
func (g sqlserverDDL) dropConstraintBy(t, query string) string {
	return strings.Join([]string{
		"DECLARE @n sysname = (" + query + ");",
		"IF @n IS NOT NULL",
		"BEGIN",
		"  DECLARE @s nvarchar(max) = N'ALTER TABLE ' + " + nLiteral(t) + " + N' DROP CONSTRAINT ' + QUOTENAME(@n);",
		"  EXEC sp_executesql @s;",
		"END",
	}, "\n")
}

func nLiteral(s string) string {
	return "N" + stringLiteral(s)
}
