package main

import (
	"strings"
	"testing"
)

func TestBuildDatabaseURL(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    string
		wantErr string
	}{
		{
			name: "all components present, options included",
			env: map[string]string{
				"DB_HOST":     "pgbouncer.java-tasks.svc.cluster.local",
				"DB_PORT":     "6432",
				"DB_NAME":     "projectordb",
				"DB_USER":     "taskuser",
				"DB_PASSWORD": "taskpass",
				"DB_OPTIONS":  "sslmode=disable&application_name=order-projector",
			},
			want: "postgres://taskuser:taskpass@pgbouncer.java-tasks.svc.cluster.local:6432/projectordb?sslmode=disable&application_name=order-projector",
		},
		{
			name: "options omitted when DB_OPTIONS empty",
			env: map[string]string{
				"DB_HOST":     "host",
				"DB_PORT":     "5432",
				"DB_NAME":     "db",
				"DB_USER":     "u",
				"DB_PASSWORD": "p",
			},
			want: "postgres://u:p@host:5432/db",
		},
		{
			name: "URL-special characters in password are escaped",
			env: map[string]string{
				"DB_HOST":     "host",
				"DB_PORT":     "5432",
				"DB_NAME":     "db",
				"DB_USER":     "user",
				"DB_PASSWORD": "p@ss/word+1=",
			},
			// QueryEscape encodes @ as %40, / as %2F, + as %2B, = as %3D.
			want: "postgres://user:p%40ss%2Fword%2B1%3D@host:5432/db",
		},
		{
			name: "missing DB_PASSWORD reports the missing key",
			env: map[string]string{
				"DB_HOST": "host",
				"DB_PORT": "5432",
				"DB_NAME": "db",
				"DB_USER": "u",
			},
			wantErr: "DB_PASSWORD",
		},
		{
			name:    "empty environment reports the first missing key",
			env:     map[string]string{},
			wantErr: "DB_HOST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear any inherited values from the test process.
			for _, k := range []string{"DB_HOST", "DB_PORT", "DB_NAME", "DB_USER", "DB_PASSWORD", "DB_OPTIONS"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got, err := buildDatabaseURL()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil; result=%q", tt.wantErr, got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("buildDatabaseURL\n got: %s\nwant: %s", got, tt.want)
			}
		})
	}
}
