package engine

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// FormSpec is the parsed representation of a .courtyard/forms/*.yaml file.
type FormSpec struct {
	Name          string                      `yaml:"name"`
	Description   string                      `yaml:"description"`
	TargetBranch  string                      `yaml:"targetBranch"`
	BranchName    string                      `yaml:"branchName"`
	BranchMode    string                      `yaml:"branchMode"`    // "reuse" (default) or "fresh"
	CommitMessage string                      `yaml:"commitMessage"`
	OutputPath    string                      `yaml:"outputPath"`
	Fields        []FieldSpec                 `yaml:"fields"`
	Templates     map[string]TemplateOverride `yaml:"templates"`
}

// FieldSpec defines a single form field.
type FieldSpec struct {
	Name       string   `yaml:"name"`
	Type       string   `yaml:"type"`       // string | number | boolean | enum
	Label      string   `yaml:"label"`
	Required   bool     `yaml:"required"`
	Default    interface{} `yaml:"default"`
	Options    []string `yaml:"options"`    // for enum type
	Validation string   `yaml:"validation"` // regex
}

// TemplateOverride allows a per-file output path override.
type TemplateOverride struct {
	OutputPath string `yaml:"outputPath"`
}

// ParseFormSpec parses raw YAML bytes into a FormSpec.
// It applies default values and validates required fields.
func ParseFormSpec(raw []byte) (*FormSpec, error) {
	var spec FormSpec
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		return nil, fmt.Errorf("ParseFormSpec: %w", err)
	}
	if spec.Name == "" {
		return nil, fmt.Errorf("ParseFormSpec: missing required field 'name'")
	}
	if spec.BranchMode == "" {
		spec.BranchMode = "reuse"
	}
	if spec.BranchMode != "reuse" && spec.BranchMode != "fresh" {
		return nil, fmt.Errorf("ParseFormSpec: invalid branchMode %q (must be 'reuse' or 'fresh')", spec.BranchMode)
	}
	return &spec, nil
}
