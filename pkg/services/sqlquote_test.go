package services

import "testing"

func TestQuoteIdent(t *testing.T) {
	if got := quoteMy("we`ird"); got != "`we``ird`" {
		t.Errorf("quoteMy = %s", got)
	}
	if got := quotePG(`we"ird`); got != `"we""ird"` {
		t.Errorf("quotePG = %s", got)
	}
}

func TestEscapeLike(t *testing.T) {
	if got := escapeLike(`50%_a\b`); got != `50\%\_a\\b` {
		t.Errorf("escapeLike = %s", got)
	}
}

func TestReturnsRows(t *testing.T) {
	rows := []string{
		"SELECT 1",
		"  select * from t",
		"(SELECT 1) UNION (SELECT 2)",
		"WITH x AS (SELECT 1) SELECT * FROM x",
		"-- comment\nSELECT 1",
		"# mysql comment\nSHOW TABLES",
		"/* hint */ EXPLAIN SELECT 1",
		"DESCRIBE users",
		"show tables",
	}
	for _, q := range rows {
		if !returnsRows(q) {
			t.Errorf("returnsRows(%q) = false, want true", q)
		}
	}
	noRows := []string{
		"INSERT INTO t VALUES (1)",
		"UPDATE t SET a = 1",
		"DELETE FROM t",
		"CREATE TABLE t (id int)",
		"-- only a comment",
		"",
		"SELECTED_THING = 1", // not the SELECT keyword
	}
	for _, q := range noRows {
		if returnsRows(q) {
			t.Errorf("returnsRows(%q) = true, want false", q)
		}
	}
}
