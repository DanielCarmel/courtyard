package engine

import (
	"context"
	"strings"
	"testing"
)

var sampleYAML = []byte(`
name: deploy-app
description: Deploy a new app
targetBranch: main
branchName: "courtyard/{{ .appName }}/{{ .environment }}"
branchMode: reuse
commitMessage: "feat: deploy {{ .appName }} to {{ .environment }}"
outputPath: "clusters/{{ .environment }}/{{ .namespace }}"

fields:
  - name: appName
    type: string
    label: Application Name
    required: true
    validation: "^[a-z][a-z0-9-]*$"
  - name: environment
    type: enum
    label: Environment
    options: [dev, staging, prod]
    required: true
  - name: replicas
    type: number
    label: Replica Count
    default: 3
  - name: enableIngress
    type: boolean
    label: Enable Ingress
    default: false

templates:
  deployment.yaml.tmpl: {}
  values.tf.tmpl:
    outputPath: "terraform/{{ .environment }}/{{ .appName }}.tf"
`)

func TestParseFormSpec(t *testing.T) {
	t.Run("valid spec", func(t *testing.T) {
		spec, err := ParseFormSpec(sampleYAML)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if spec.Name != "deploy-app" {
			t.Errorf("name: got %q, want %q", spec.Name, "deploy-app")
		}
		if spec.BranchMode != "reuse" {
			t.Errorf("branchMode: got %q, want %q", spec.BranchMode, "reuse")
		}
		if len(spec.Fields) != 4 {
			t.Errorf("fields: got %d, want 4", len(spec.Fields))
		}
		if spec.Templates["values.tf.tmpl"].OutputPath == "" {
			t.Error("expected non-empty outputPath override for values.tf.tmpl")
		}
	})

	t.Run("default branchMode", func(t *testing.T) {
		spec, err := ParseFormSpec([]byte("name: test\n"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if spec.BranchMode != "reuse" {
			t.Errorf("default branchMode: got %q, want 'reuse'", spec.BranchMode)
		}
	})

	t.Run("missing name returns error", func(t *testing.T) {
		_, err := ParseFormSpec([]byte("description: no name\n"))
		if err == nil {
			t.Fatal("expected error for missing name")
		}
	})

	t.Run("invalid branchMode returns error", func(t *testing.T) {
		_, err := ParseFormSpec([]byte("name: test\nbranchMode: invalid\n"))
		if err == nil {
			t.Fatal("expected error for invalid branchMode")
		}
	})

	t.Run("malformed YAML returns error", func(t *testing.T) {
		_, err := ParseFormSpec([]byte("name: [\ninvalid"))
		if err == nil {
			t.Fatal("expected error for malformed YAML")
		}
	})
}

func TestEvaluate(t *testing.T) {
	spec, err := ParseFormSpec(sampleYAML)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	values := map[string]interface{}{
		"appName":     "myapp",
		"environment": "dev",
		"namespace":   "default",
	}

	templateFiles := map[string][]byte{
		"deployment.yaml.tmpl": []byte("app: {{ .appName }}\nenv: {{ .environment }}\n"),
		"values.tf.tmpl":       []byte(`resource "app" "{{ .appName }}" {}`),
	}

	t.Run("renders all files", func(t *testing.T) {
		files, err := Evaluate(context.Background(), spec, templateFiles, values)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if len(files) != 2 {
			t.Fatalf("expected 2 output files, got %d", len(files))
		}
	})

	t.Run("default output path applied", func(t *testing.T) {
		files, err := Evaluate(context.Background(), spec, templateFiles, values)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		var foundDeployment bool
		for _, f := range files {
			if strings.HasSuffix(f.Path, "deployment.yaml") {
				foundDeployment = true
				if !strings.HasPrefix(f.Path, "clusters/dev/default/") {
					t.Errorf("deployment path: got %q, expected prefix clusters/dev/default/", f.Path)
				}
			}
		}
		if !foundDeployment {
			t.Error("deployment.yaml not found in output files")
		}
	})

	t.Run("per-file output path override applied", func(t *testing.T) {
		files, err := Evaluate(context.Background(), spec, templateFiles, values)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		var foundTF bool
		for _, f := range files {
			if strings.HasSuffix(f.Path, ".tf") {
				foundTF = true
				want := "terraform/dev/myapp.tf"
				if f.Path != want {
					t.Errorf("tf path: got %q, want %q", f.Path, want)
				}
			}
		}
		if !foundTF {
			t.Error("terraform file not found in output files")
		}
	})

	t.Run("sprig upper function works", func(t *testing.T) {
		files, err := Evaluate(context.Background(), spec, map[string][]byte{
			"out.txt.tmpl": []byte(`{{ .appName | upper }}`),
		}, values)
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if len(files) != 1 {
			t.Fatalf("expected 1 file, got %d", len(files))
		}
		if string(files[0].Content) != "MYAPP" {
			t.Errorf("content: got %q, want %q", string(files[0].Content), "MYAPP")
		}
	})

	t.Run("malformed template returns error", func(t *testing.T) {
		_, err := Evaluate(context.Background(), spec, map[string][]byte{
			"bad.tmpl": []byte(`{{ .foo `),
		}, values)
		if err == nil {
			t.Fatal("expected error for malformed template")
		}
	})
}

func TestEvaluateString(t *testing.T) {
	tests := []struct {
		name   string
		tmpl   string
		values map[string]interface{}
		want   string
		wantErr bool
	}{
		{
			name:   "simple substitution",
			tmpl:   "courtyard/{{ .app }}/{{ .env }}",
			values: map[string]interface{}{"app": "myapp", "env": "dev"},
			want:   "courtyard/myapp/dev",
		},
		{
			name:   "sprig default function",
			tmpl:   `{{ default "fallback" .missing }}`,
			values: map[string]interface{}{},
			want:   "fallback",
		},
		{
			name:    "malformed template",
			tmpl:    "{{ .foo ",
			values:  map[string]interface{}{},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := EvaluateString(tc.tmpl, tc.values)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateValues(t *testing.T) {
	spec, err := ParseFormSpec(sampleYAML)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	tests := []struct {
		name     string
		values   map[string]interface{}
		wantErrs []string // field names that should have errors
	}{
		{
			name: "all valid",
			values: map[string]interface{}{
				"appName":     "myapp",
				"environment": "dev",
			},
			wantErrs: nil,
		},
		{
			name:     "missing required fields",
			values:   map[string]interface{}{},
			wantErrs: []string{"appName", "environment"},
		},
		{
			name: "invalid enum value",
			values: map[string]interface{}{
				"appName":     "myapp",
				"environment": "production", // not in options
			},
			wantErrs: []string{"environment"},
		},
		{
			name: "invalid regex",
			values: map[string]interface{}{
				"appName":     "MyApp", // fails ^[a-z][a-z0-9-]*$
				"environment": "dev",
			},
			wantErrs: []string{"appName"},
		},
		{
			name: "invalid number type",
			values: map[string]interface{}{
				"appName":     "myapp",
				"environment": "dev",
				"replicas":    "not-a-number",
			},
			wantErrs: []string{"replicas"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateValues(spec, tc.values)
			for _, field := range tc.wantErrs {
				if _, ok := errs[field]; !ok {
					t.Errorf("expected error for field %q, got none", field)
				}
			}
			for field := range errs {
				found := false
				for _, want := range tc.wantErrs {
					if field == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("unexpected error for field %q: %s", field, errs[field])
				}
			}
		})
	}
}
