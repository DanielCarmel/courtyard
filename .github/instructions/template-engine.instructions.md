---
description: "Template engine patterns for Courtyard. Use when working on form parsing, template evaluation, or the preview/submit flow in pkg/engine/. Covers Sprig integration, output path resolution, and validation."
applyTo: "pkg/engine/**"
---
# Template Engine

## Form Spec Parsing (`parser.go`)

- Parse `.courtyard/forms/*.yaml` with `gopkg.in/yaml.v3`
- Use `decoder.KnownFields(true)` for strict validation (reject unknown YAML keys)
- Validate required fields: `name`, `fields` (at minimum)
- Default `branchMode` to `"reuse"` if omitted
- Default `targetBranch` to empty (handler will use repo's default branch)

## FormSpec YAML Structure

```yaml
name: string           # required
description: string    # optional
targetBranch: string   # optional (defaults to repo default branch)
branchName: string     # Go template for PR branch name
branchMode: string     # "reuse" | "fresh" (default: "reuse")
commitMessage: string  # Go template for commit message
outputPath: string     # Go template for default output directory
fields: []FieldSpec    # required, at least one field
templates: map         # optional per-file output path overrides
```

## Template Evaluation (`evaluator.go`)

- Function signature: `Evaluate(spec *FormSpec, templateFiles map[string][]byte, values map[string]interface{}) ([]OutputFile, error)`
- Always call `sprig.TxtFuncMap()` (not `FuncMap()` — that's for html/template)
- Set FuncMap BEFORE calling `Parse()`:
  ```go
  tmpl := template.New(name).Funcs(sprig.TxtFuncMap())
  tmpl, err := tmpl.Parse(string(content))
  ```
- Execute with 5-second context deadline — cancel if exceeded
- Strip `.tmpl` suffix from filenames for output path

## Output Path Resolution

For each template file:
1. Check `spec.Templates[filename]` for a per-file `OutputPath` override
2. If override exists: evaluate it as a Go template with user values
3. If no override: evaluate `spec.OutputPath` template + append relative path (sans `.tmpl`)
4. Preserve nested directory structure from the templates directory

## Error Handling

- Aggregate ALL errors — don't stop on first template failure
- Return a multi-error (slice of errors) so the UI can show all issues
- Never panic on malformed templates
- Template parse errors and execution errors are both returned gracefully
- Include the template filename in each error message

## Validation (Server-Side)

Before evaluating templates, validate all field values against `FieldSpec`:
- `required`: reject empty/zero values
- `type`: coerce and validate (string, number, boolean, enum)
- `validation`: compile regex once, match against string value
- `options`: for enum type, reject values not in the options list
- Return all validation errors at once (not first-only)

## Helper Functions

Provide separate helpers for evaluating:
- `EvaluateBranchName(spec *FormSpec, values map[string]interface{}) (string, error)`
- `EvaluateCommitMessage(spec *FormSpec, values map[string]interface{}) (string, error)`

These use the same Sprig FuncMap and error handling patterns.
