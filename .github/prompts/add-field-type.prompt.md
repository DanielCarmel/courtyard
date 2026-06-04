---
description: "Add a new form field type to Courtyard. Updates the FieldSpec type enum, frontend renderer, backend validator, and adds tests."
agent: "courtyard"
argument-hint: "Field type name and behavior (e.g., 'multiline - textarea for long text')"
---
# Add Form Field Type

Add a new field type to the Courtyard form system. This touches four layers:

## Steps

1. **Update `pkg/engine/parser.go`**
   - Add the new type to the `FieldSpec.Type` documentation/constants
   - Add any new fields to `FieldSpec` struct if the type needs them (e.g., `MaxLength`, `Rows`)
   - Update YAML parsing if new struct fields are added

2. **Update `pkg/engine/validator.go`**
   - Add a validation case for the new type in the field validation function
   - Define what "valid" means for this type (e.g., multiline: non-empty string if required)
   - Add type coercion if needed (e.g., string → []string for multi-select)

3. **Update `ui/static/app.js`**
   - Add a rendering case in the form field renderer
   - Map the type to the appropriate HTML element(s)
   - Wire up Alpine.js bindings (`x-model`, validation attributes)
   - Ensure the value is included in preview/submit payloads

4. **Write tests**
   - `pkg/engine/validator_test.go`: Valid and invalid inputs for the new type
   - `pkg/engine/parser_test.go`: YAML with the new type parses correctly
   - `pkg/engine/evaluator_test.go`: Templates can use the new field value

5. **Verify**: `go build ./... && go vet ./... && go test ./...`

## Existing Types for Reference

| Type | HTML Element | Validation |
|------|-------------|------------|
| `string` | `<input type="text">` | Required, regex pattern |
| `number` | `<input type="number">` | Required, numeric |
| `boolean` | `<input type="checkbox">` | (always valid) |
| `enum` | `<select>` | Required, value in options list |

## Template Usage

Field values are passed to Go templates as `map[string]interface{}`. The new type's value must be usable in templates:
- Strings: `{{ .fieldName }}`
- Numbers: `{{ .fieldName }}`
- Booleans: `{{ if .fieldName }}...{{ end }}`
- Lists: `{{ range .fieldName }}...{{ end }}`
