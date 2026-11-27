package schema

import (
	"reflect"
	"slices"
	"testing"

	"github.com/lichenghsu/synccraft/internal/dialect"
)

func col(pos int, name, typ string, nullable bool) dialect.Column {
	return dialect.Column{Name: name, Type: typ, Nullable: nullable, Position: pos}
}

func users(cols ...dialect.Column) Table {
	return Table{Table: dialect.Table{Name: "users"}, Columns: cols, PrimaryKey: []string{"id"}, Indexes: []dialect.Index{}}
}

var id = col(1, "id", "int", false)

// summary is a compact view of a change for table-driven tests.
type summary struct {
	Kind   Kind
	Table  string
	Detail string
	After  string
	Danger bool
}

func summarize(cs []Change) []summary {
	out := []summary{}
	for _, c := range cs {
		s := summary{Kind: c.Kind, Table: c.Table.Name, After: c.After, Danger: c.Destructive}
		switch {
		case c.Column != nil:
			s.Detail = c.Column.Name
		case c.Index != nil:
			s.Detail = c.Index.Name
		}
		out = append(out, s)
	}
	return out
}

func TestDiff(t *testing.T) {
	tests := []struct {
		name     string
		src, dst []Table
		want     []summary
	}{
		{
			name: "identical schemas have no changes",
			src:  []Table{users(id, col(2, "email", "varchar(255)", false))},
			dst:  []Table{users(id, col(2, "email", "varchar(255)", false))},
			want: []summary{},
		},
		{
			name: "new and dropped tables",
			src:  []Table{users(id), {Table: dialect.Table{Name: "orders"}, Columns: []dialect.Column{id}}},
			dst:  []Table{users(id), {Table: dialect.Table{Name: "audit"}, Columns: []dialect.Column{id}}},
			want: []summary{
				{Kind: AddTable, Table: "orders"},
				{Kind: DropTable, Table: "audit", Danger: true},
			},
		},
		{
			name: "tables match by schema and name",
			src:  []Table{{Table: dialect.Table{Schema: "sales", Name: "users"}, Columns: []dialect.Column{id}}},
			dst:  []Table{{Table: dialect.Table{Schema: "public", Name: "users"}, Columns: []dialect.Column{id}}},
			want: []summary{
				{Kind: AddTable, Table: "users"},
				{Kind: DropTable, Table: "users", Danger: true},
			},
		},
		{
			name: "added columns name the preceding source column, in position order",
			src: []Table{users(
				col(3, "nick", "varchar(50)", true),
				id,
				col(2, "email", "varchar(255)", false),
			)},
			dst: []Table{users(id)},
			want: []summary{
				{Kind: AddColumn, Table: "users", Detail: "email", After: "id"},
				{Kind: AddColumn, Table: "users", Detail: "nick", After: "email"},
			},
		},
		{
			name: "column added first has empty After",
			src:  []Table{users(col(1, "tenant", "int", false), col(2, "id", "int", false))},
			dst:  []Table{users(id)},
			want: []summary{{Kind: AddColumn, Table: "users", Detail: "tenant"}},
		},
		{
			name: "dropped column is destructive",
			src:  []Table{users(id)},
			dst:  []Table{users(id, col(2, "legacy", "text", true))},
			want: []summary{{Kind: DropColumn, Table: "users", Detail: "legacy", Danger: true}},
		},
		{
			name: "type change is destructive, nullability change is not",
			src:  []Table{users(id, col(2, "email", "varchar(255)", false), col(3, "nick", "text", true))},
			dst:  []Table{users(id, col(2, "email", "varchar(100)", false), col(3, "nick", "text", false))},
			want: []summary{
				{Kind: AlterColumn, Table: "users", Detail: "email", Danger: true},
				{Kind: AlterColumn, Table: "users", Detail: "nick"},
			},
		},
		{
			name: "position change alone is not a difference",
			src:  []Table{users(col(1, "email", "text", true), col(2, "id", "int", false))},
			dst:  []Table{users(col(1, "id", "int", false), col(2, "email", "text", true))},
			want: []summary{},
		},
		{
			name: "normalized types and defaults are equal",
			src: []Table{users(
				col(1, "id", "INT(11)", false),
				dialect.Column{Name: "role", Type: "varchar(20)", Nullable: true, Default: new("'guest'::character varying"), Position: 2},
			)},
			dst: []Table{users(
				col(1, "id", "int", false),
				dialect.Column{Name: "role", Type: "VARCHAR(20)", Nullable: true, Default: new("'guest'"), Position: 2},
			)},
			want: []summary{},
		},
		{
			name: "default added or removed",
			src:  []Table{users(id, dialect.Column{Name: "n", Type: "int", Default: new("0"), Position: 2})},
			dst:  []Table{users(id, dialect.Column{Name: "n", Type: "int", Position: 2})},
			want: []summary{{Kind: AlterColumn, Table: "users", Detail: "n"}},
		},
		{
			name: "changed index is dropped then added after columns",
			src: []Table{{
				Table: dialect.Table{Name: "users"}, Columns: []dialect.Column{id, col(2, "email", "text", false)},
				Indexes: []dialect.Index{{Name: "ix_email", Columns: []string{"email"}, Unique: true}},
			}},
			dst: []Table{{
				Table: dialect.Table{Name: "users"}, Columns: []dialect.Column{id},
				Indexes: []dialect.Index{{Name: "ix_email", Columns: []string{"id"}}},
			}},
			want: []summary{
				{Kind: DropIndex, Table: "users", Detail: "ix_email"},
				{Kind: AddColumn, Table: "users", Detail: "email", After: "id"},
				{Kind: AddIndex, Table: "users", Detail: "ix_email"},
			},
		},
		{
			name: "unique constraint vs unique index differ",
			src: []Table{{Table: dialect.Table{Name: "t"}, Columns: []dialect.Column{id},
				Indexes: []dialect.Index{{Name: "u", Columns: []string{"id"}, Unique: true, Constraint: true}}}},
			dst: []Table{{Table: dialect.Table{Name: "t"}, Columns: []dialect.Column{id},
				Indexes: []dialect.Index{{Name: "u", Columns: []string{"id"}, Unique: true}}}},
			want: []summary{
				{Kind: DropIndex, Table: "t", Detail: "u"},
				{Kind: AddIndex, Table: "t", Detail: "u"},
			},
		},
		{
			name: "primary key change sits between column changes and column drops",
			src: []Table{{Table: dialect.Table{Name: "t"},
				Columns:    []dialect.Column{id, col(2, "tenant", "int", false)},
				PrimaryKey: []string{"tenant", "id"}}},
			dst: []Table{{Table: dialect.Table{Name: "t"},
				Columns:    []dialect.Column{id, col(2, "code", "int", false)},
				PrimaryKey: []string{"code"}}},
			want: []summary{
				{Kind: AddColumn, Table: "t", Detail: "tenant", After: "id"},
				{Kind: AlterPrimaryKey, Table: "t"},
				{Kind: DropColumn, Table: "t", Detail: "code", Danger: true},
			},
		},
		{
			name: "nil and empty primary key are equal",
			src:  []Table{{Table: dialect.Table{Name: "t"}, Columns: []dialect.Column{id}}},
			dst:  []Table{{Table: dialect.Table{Name: "t"}, Columns: []dialect.Column{id}, PrimaryKey: []string{}}},
			want: []summary{},
		},
		{
			name: "changes grouped by table, tables sorted",
			src:  []Table{users(id, col(2, "a", "int", true)), {Table: dialect.Table{Name: "accounts"}, Columns: []dialect.Column{id, col(2, "b", "int", true)}}},
			dst:  []Table{users(id), {Table: dialect.Table{Name: "accounts"}, Columns: []dialect.Column{id}}},
			want: []summary{
				{Kind: AddColumn, Table: "accounts", Detail: "b", After: "id"},
				{Kind: AddColumn, Table: "users", Detail: "a", After: "id"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarize(Diff(tt.src, tt.dst))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Diff:\n got  %+v\n want %+v", got, tt.want)
			}
		})
	}
}

func TestDiffCarriesDefinitions(t *testing.T) {
	src := []Table{users(id, col(2, "email", "varchar(255)", false))}
	dst := []Table{users(id, col(2, "email", "varchar(100)", true))}
	got := Diff(src, dst)
	if len(got) != 1 {
		t.Fatalf("got %d changes, want 1", len(got))
	}
	c := got[0]
	if c.Column.Type != "varchar(255)" || c.From.Type != "varchar(100)" || !c.From.Nullable {
		t.Errorf("AlterColumn carries Column %+v From %+v", c.Column, c.From)
	}

	got = Diff([]Table{users(id)}, nil)
	if got[0].Create == nil || got[0].Create.Columns[0].Name != "id" {
		t.Errorf("AddTable Create = %+v", got[0].Create)
	}
}

func TestDiffDoesNotReorderInputs(t *testing.T) {
	src := []Table{
		users(col(2, "b", "int", true), col(1, "a", "int", true)),
		{Table: dialect.Table{Name: "accounts"}},
	}
	before := []string{src[0].Table.Name, src[1].Table.Name, src[0].Columns[0].Name}
	Diff(src, nil)
	after := []string{src[0].Table.Name, src[1].Table.Name, src[0].Columns[0].Name}
	if !slices.Equal(before, after) {
		t.Errorf("inputs reordered: %v -> %v", before, after)
	}
}

func TestDiffNeverNil(t *testing.T) {
	if got := Diff(nil, nil); got == nil {
		t.Error("Diff(nil, nil) = nil, want empty slice")
	}
}
