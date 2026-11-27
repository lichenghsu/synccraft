package schema

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"

	"github.com/lichenghsu/synccraft/internal/dialect"
)

// roundTrip applies Diff(src, dst) to dst and checks that comparing again
// finds nothing. It returns the applied steps.
func roundTrip(t *testing.T, d dialect.Dialect, src, dst *sql.DB) []Step {
	t.Helper()
	ctx := context.Background()
	s, err := Load(ctx, d, src)
	if err != nil {
		t.Fatalf("load source: %v", err)
	}
	before, err := Load(ctx, d, dst)
	if err != nil {
		t.Fatalf("load target: %v", err)
	}
	changes := Diff(s, before)
	if len(changes) == 0 {
		t.Fatal("fixture has no differences")
	}
	steps, err := Script(d, before, changes)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range steps {
		if st.Unsupported != "" {
			t.Fatalf("%s %s unsupported: %s", st.Change.Kind, st.Change.Table.Name, st.Unsupported)
		}
	}
	if err := Apply(ctx, dst, steps); err != nil {
		t.Fatalf("apply: %v\nscript:\n%s", err, formatSteps(steps))
	}
	after, err := Load(ctx, d, dst)
	if err != nil {
		t.Fatalf("reload target: %v", err)
	}
	if again := Diff(s, after); len(again) != 0 {
		t.Errorf("differences after apply:\n%+v\nscript:\n%s", summarize(again), formatSteps(steps))
	}
	return steps
}

func formatSteps(steps []Step) string {
	var b strings.Builder
	for _, s := range steps {
		b.WriteString("-- " + string(s.Change.Kind) + " " + s.Change.Table.Name + "\n")
		for _, stmt := range s.SQL {
			b.WriteString(stmt + ";\n")
		}
	}
	return b.String()
}

func TestSQLiteRoundTrip(t *testing.T) {
	src := openSQLite(t,
		`CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			email TEXT NOT NULL,
			nick TEXT DEFAULT 'guest',
			score INTEGER NOT NULL DEFAULT 0,
			created_at TEXT DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT uq_email UNIQUE (email)
		)`,
		`CREATE INDEX ix_nick ON users (nick)`,
		`CREATE TABLE orders (id INTEGER PRIMARY KEY, user_id INTEGER, total NUMERIC(10,2))`,
		`CREATE INDEX ix_orders_user ON orders (user_id)`,
		`CREATE TABLE tags (post INTEGER NOT NULL, tag TEXT NOT NULL, PRIMARY KEY (post, tag))`,
	)
	dst := openSQLite(t,
		`CREATE TABLE users (id INTEGER PRIMARY KEY, email VARCHAR(100), legacy TEXT)`,
		`CREATE INDEX ix_nick ON users (email)`,
		`CREATE INDEX ix_legacy ON users (legacy)`,
		`INSERT INTO users (id, email, legacy) VALUES (1, 'a@x', 'old'), (2, 'b@x', NULL)`,
		`CREATE TABLE tags (post INTEGER NOT NULL, tag TEXT NOT NULL, PRIMARY KEY (post))`,
		`INSERT INTO tags VALUES (1, 'go')`,
		`CREATE TABLE audit (id INTEGER)`,
	)
	roundTrip(t, dialect.SQLite{}, src, dst)

	// Rebuilds keep the rows of shared columns.
	var n int
	var email, nick string
	if err := dst.QueryRow(`SELECT COUNT(*), MIN(email), MIN(nick) FROM users`).Scan(&n, &email, &nick); err != nil {
		t.Fatal(err)
	}
	if n != 2 || email != "a@x" || nick != "guest" {
		t.Errorf("users after rebuild: count %d, email %q, nick %q", n, email, nick)
	}
	if err := dst.QueryRow(`SELECT COUNT(*) FROM tags`).Scan(&n); err != nil || n != 1 {
		t.Errorf("tags after rebuild: %d, %v", n, err)
	}
}

func TestScriptStatements(t *testing.T) {
	users := dialect.Table{Name: "users"}
	email := dialect.Column{Name: "email", Type: "varchar(255)", Nullable: true, Position: 2}
	tests := []struct {
		name string
		d    dialect.Dialect
		c    Change
		want []string
	}{
		{
			name: "mysql add column after",
			d:    dialect.MySQL{},
			c:    Change{Kind: AddColumn, Table: users, Column: &email, After: "id"},
			want: []string{"ALTER TABLE `users` ADD COLUMN `email` varchar(255) AFTER `id`"},
		},
		{
			name: "mysql add first column",
			d:    dialect.MySQL{},
			c:    Change{Kind: AddColumn, Table: users, Column: &email},
			want: []string{"ALTER TABLE `users` ADD COLUMN `email` varchar(255) FIRST"},
		},
		{
			name: "mysql expression default is parenthesized",
			d:    dialect.MySQL{},
			c:    Change{Kind: AddColumn, Table: users, Column: &dialect.Column{Name: "u", Type: "char(36)", Nullable: true, Default: new("uuid()")}, After: "id"},
			want: []string{"ALTER TABLE `users` ADD COLUMN `u` char(36) DEFAULT (uuid()) AFTER `id`"},
		},
		{
			name: "postgres serial default becomes serial type",
			d:    dialect.PostgreSQL{},
			c: Change{Kind: AddColumn, Table: dialect.Table{Schema: "s", Name: "t"},
				Column: &dialect.Column{Name: "id", Type: "bigint", Default: new("nextval('s.t_id_seq'::regclass)")}},
			want: []string{`ALTER TABLE "s"."t" ADD COLUMN "id" bigserial NOT NULL`},
		},
		{
			name: "postgres drop index is schema qualified",
			d:    dialect.PostgreSQL{},
			c:    Change{Kind: DropIndex, Table: dialect.Table{Schema: "s", Name: "t"}, Index: &dialect.Index{Name: "ix"}},
			want: []string{`DROP INDEX "s"."ix"`},
		},
		{
			name: "postgres alter column type, default and nullability",
			d:    dialect.PostgreSQL{},
			c: Change{Kind: AlterColumn, Table: users,
				From:   &dialect.Column{Name: "n", Type: "integer", Nullable: true},
				Column: &dialect.Column{Name: "n", Type: "bigint", Default: new("0")}},
			want: []string{
				`ALTER TABLE "users" ALTER COLUMN "n" TYPE bigint USING "n"::bigint`,
				`ALTER TABLE "users" ALTER COLUMN "n" SET DEFAULT 0`,
				`ALTER TABLE "users" ALTER COLUMN "n" SET NOT NULL`,
			},
		},
		{
			name: "sqlserver nullability change keeps the default",
			d:    dialect.SQLServer{},
			c: Change{Kind: AlterColumn, Table: users,
				From:   &dialect.Column{Name: "n", Type: "int", Nullable: true, Default: new("0")},
				Column: &dialect.Column{Name: "n", Type: "int", Default: new("0")}},
			want: []string{
				new(sqlserverDDL{}).dropDefault("[users]", "n"),
				"ALTER TABLE [users] ALTER COLUMN [n] int NOT NULL",
				"ALTER TABLE [users] ADD DEFAULT 0 FOR [n]",
			},
		},
		{
			name: "sqlserver default-only change does not alter the column",
			d:    dialect.SQLServer{},
			c: Change{Kind: AlterColumn, Table: users,
				From:   &dialect.Column{Name: "n", Type: "int", Nullable: true},
				Column: &dialect.Column{Name: "n", Type: "int", Nullable: true, Default: new("1")}},
			want: []string{"ALTER TABLE [users] ADD DEFAULT 1 FOR [n]"},
		},
		{
			name: "sqlite nullable column is added in place",
			d:    dialect.SQLite{},
			c:    Change{Kind: AddColumn, Table: users, Column: &dialect.Column{Name: "nick", Type: "TEXT", Nullable: true}},
			want: []string{`ALTER TABLE "users" ADD COLUMN "nick" TEXT`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dst := []Table{{Table: tt.c.Table, Columns: []dialect.Column{{Name: "id", Type: "int", Position: 1}}}}
			steps, err := Script(tt.d, dst, []Change{tt.c})
			if err != nil {
				t.Fatal(err)
			}
			if steps[0].Unsupported != "" {
				t.Fatalf("unsupported: %s", steps[0].Unsupported)
			}
			if !reflect.DeepEqual(steps[0].SQL, tt.want) {
				t.Errorf("SQL =\n%q\nwant\n%q", steps[0].SQL, tt.want)
			}
		})
	}
}

func TestScriptUnsupported(t *testing.T) {
	users := dialect.Table{Name: "users"}
	dst := []Table{{Table: users, Columns: []dialect.Column{{Name: "id", Type: "int", Position: 1}}}}
	tests := []struct {
		name string
		d    dialect.Dialect
		c    Change
	}{
		{"table missing in target", dialect.MySQL{}, Change{Kind: DropColumn, Table: dialect.Table{Name: "nope"}, Column: &dialect.Column{Name: "x"}}},
		{"table already exists", dialect.MySQL{}, Change{Kind: AddTable, Table: users, Create: &dst[0]}},
		{"expression index", dialect.PostgreSQL{}, Change{Kind: AddIndex, Table: users, Index: &dialect.Index{Name: "ix", Columns: []string{""}}}},
		{"sqlserver identity change", dialect.SQLServer{}, Change{Kind: AlterColumn, Table: users,
			From: &dst[0].Columns[0], Column: &dialect.Column{Name: "id", Type: "int", AutoIncrement: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps, err := Script(tt.d, dst, []Change{tt.c})
			if err != nil {
				t.Fatal(err)
			}
			if steps[0].Unsupported == "" || len(steps[0].SQL) != 0 {
				t.Errorf("step = %+v, want unsupported with no SQL", steps[0])
			}
			if err := Apply(context.Background(), nil, steps); err == nil {
				t.Error("Apply accepted an unsupported step")
			}
		})
	}
}

func TestScriptSubsetSeesEarlierSteps(t *testing.T) {
	// A rebuild after an added column must keep that column.
	users := dialect.Table{Name: "users"}
	dst := []Table{{Table: users, Columns: []dialect.Column{{Name: "id", Type: "INTEGER", Position: 1}}}}
	nick := dialect.Column{Name: "nick", Type: "TEXT", Nullable: true, Position: 2}
	steps, err := Script(dialect.SQLite{}, dst, []Change{
		{Kind: AddColumn, Table: users, Column: &nick, After: "id"},
		{Kind: AlterPrimaryKey, Table: users, PrimaryKey: []string{"id"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	create := steps[1].SQL[0]
	if !strings.Contains(create, `"nick" TEXT`) || !strings.Contains(create, `PRIMARY KEY ("id")`) {
		t.Errorf("rebuild CREATE = %s", create)
	}
	if !strings.Contains(steps[1].SQL[1], `("id", "nick")`) {
		t.Errorf("rebuild copies %s, want both columns", steps[1].SQL[1])
	}
	// Script must not modify dst.
	if len(dst[0].Columns) != 1 || dst[0].PrimaryKey != nil {
		t.Errorf("dst modified: %+v", dst[0])
	}
}

func TestTransactionalDDL(t *testing.T) {
	for d, want := range map[dialect.Dialect]bool{
		dialect.SQLite{}: true, dialect.PostgreSQL{}: true, dialect.SQLServer{}: true, dialect.MySQL{}: false,
	} {
		if got := TransactionalDDL(d); got != want {
			t.Errorf("TransactionalDDL(%T) = %v", d, got)
		}
	}
}
