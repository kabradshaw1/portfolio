package lint

import "testing"

func TestParseFile_SingleCreateTable(t *testing.T) {
	src := []byte("CREATE TABLE users (id UUID PRIMARY KEY);")
	ctx, err := parseSource("inline.sql", src)
	if err != nil {
		t.Fatalf("parseSource: %v", err)
	}
	if got := len(ctx.Statements); got != 1 {
		t.Fatalf("expected 1 statement, got %d", got)
	}
	if ctx.Filename != "inline.sql" {
		t.Errorf("Filename: got %q want inline.sql", ctx.Filename)
	}
}

func TestParseFile_SyntaxError(t *testing.T) {
	src := []byte("CREATE TABBLE users (id UUID);")
	if _, err := parseSource("bad.sql", src); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}
