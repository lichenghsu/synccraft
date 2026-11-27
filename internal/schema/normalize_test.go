package schema

import "testing"

func TestNormalizeType(t *testing.T) {
	tests := []struct{ in, want string }{
		{"VARCHAR(20)", "varchar(20)"},
		{"int(11)", "int"},
		{"int(10) unsigned", "int unsigned"},
		{"bigint(20)", "bigint"},
		{"tinyint(1)", "tinyint"},
		{"decimal(10,2)", "decimal(10,2)"},
		{"  character   varying(20) ", "character varying(20)"},
		{"interval(3)", "interval(3)"},
	}
	for _, tt := range tests {
		if got := normalizeType(tt.in); got != tt.want {
			t.Errorf("normalizeType(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeDefault(t *testing.T) {
	tests := []struct{ in, want string }{
		{"'guest'::character varying", "'guest'"},
		{"'2020-01-01'::timestamp without time zone", "'2020-01-01'"},
		{"'{}'::integer[]", "'{}'"},
		{"'1.50'::numeric(10,2)", "'1.50'"},
		{"nextval('t_id_seq'::regclass)", "nextval('t_id_seq'::regclass)"},
		{"((0))", "0"},
		{"(getdate())", "getdate()"},
		{"(1) + (2)", "(1) + (2)"},
		{"CURRENT_TIMESTAMP", "current_timestamp"},
		{"current_timestamp()", "current_timestamp"},
		{"'Mixed Case'", "'Mixed Case'"},
		{"'it''s ( here'", "'it''s ( here'"},
		{"'a::b'", "'a::b'"},
	}
	for _, tt := range tests {
		if got := normalizeDefault(tt.in); got != tt.want {
			t.Errorf("normalizeDefault(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
