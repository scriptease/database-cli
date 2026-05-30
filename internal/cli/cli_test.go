package cli

import (
	"testing"
)

func TestParseCollectsFlagsAndPositionals(t *testing.T) {
	parsed := parse([]string{"--alias", "prod", "--json", "SELECT 1"}, 0)

	if parsed.flags["alias"] != "prod" {
		t.Fatalf("alias = %q, want prod", parsed.flags["alias"])
	}
	if _, ok := parsed.flags["json"]; !ok {
		t.Fatal("expected --json flag to be present")
	}
	if len(parsed.positionals) != 1 || parsed.positionals[0] != "SELECT 1" {
		t.Fatalf("positionals = %#v, want [SELECT 1]", parsed.positionals)
	}
}

func TestSQLArgumentPrefersPositionalAndFallsBackToFlag(t *testing.T) {
	got, err := sqlArgument(map[string]string{"sql": "SELECT 2"}, []string{"SELECT 1"})
	if err != nil {
		t.Fatalf("sqlArgument(positionals) error = %v", err)
	}
	if got != "SELECT 1" {
		t.Fatalf("sqlArgument(positionals) = %q, want SELECT 1", got)
	}

	got, err = sqlArgument(map[string]string{"sql": "SELECT 2"}, nil)
	if err != nil {
		t.Fatalf("sqlArgument(flag) error = %v", err)
	}
	if got != "SELECT 2" {
		t.Fatalf("sqlArgument(flag) = %q, want SELECT 2", got)
	}
}

func TestInjectAliasOnlyWhenMissing(t *testing.T) {
	got, err := injectAlias(`{"op":"query","sql":"SELECT 1"}`, "prod")
	if err != nil {
		t.Fatalf("injectAlias(missing) error = %v", err)
	}
	want := `{"alias":"prod","op":"query","sql":"SELECT 1"}`
	if got != want {
		t.Fatalf("injectAlias(missing) = %q, want %q", got, want)
	}

	got, err = injectAlias(`{"alias":"existing","op":"query","sql":"SELECT 1"}`, "prod")
	if err != nil {
		t.Fatalf("injectAlias(existing) error = %v", err)
	}
	if got != `{"alias":"existing","op":"query","sql":"SELECT 1"}` {
		t.Fatalf("injectAlias(existing) = %q, want original line", got)
	}
}
