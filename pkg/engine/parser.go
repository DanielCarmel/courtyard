package engine

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// FormSpec is the parsed representation of a .courtyard/forms/*.yaml file.
type FormSpec struct {
	Name          string                      `yaml:"name"          json:"name"`
	Description   string                      `yaml:"description"   json:"description"`
	TargetBranch  string                      `yaml:"targetBranch"  json:"targetBranch"`
	BranchName    string                      `yaml:"branchName"    json:"branchName"`
	BranchMode    string                      `yaml:"branchMode"    json:"branchMode"`    // "reuse" (default) or "fresh"
	CommitMessage string                      `yaml:"commitMessage" json:"commitMessage"`
	OutputPath    string                      `yaml:"outputPath"    json:"outputPath"`
	Fields        []FieldSpec                 `yaml:"fields"        json:"fields"`
	Templates     map[string]TemplateOverride `yaml:"templates"     json:"templates"`
}

// FieldSpec defines a single form field.
type FieldSpec struct {
	Name       string      `yaml:"name"       json:"name"`
	Type       string      `yaml:"type"       json:"type"`       // string | number | boolean | enum
	Label      string      `yaml:"label"      json:"label"`
	Required   bool        `yaml:"required"   json:"required"`
	Default    interface{} `yaml:"default"    json:"default"`
	Options    []string    `yaml:"options"    json:"options"`    // for enum type
	Validation string      `yaml:"validation" json:"validation"` // regex
}

// TemplateOverride allows a per-file output path override.
type TemplateOverride struct {
	OutputPath string `yaml:"outputPath" json:"outputPath"`
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
