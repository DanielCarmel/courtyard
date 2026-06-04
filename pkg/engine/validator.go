package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ValidateValues checks that all provided values satisfy the field specs in the FormSpec.
// Returns a map of field name → error message for any invalid fields.
func ValidateValues(spec *FormSpec, values map[string]interface{}) map[string]string {
	errs := make(map[string]string)

	for _, field := range spec.Fields {
		val, present := values[field.Name]

		if field.Required && (!present || isEmpty(val)) {
			errs[field.Name] = "required"
			continue
		}
		if !present || isEmpty(val) {
			continue
		}

		switch field.Type {
		case "string":
			s, ok := toString(val)
			if !ok {
				errs[field.Name] = "must be a string"
				continue
			}
			if field.Validation != "" {
				matched, err := regexp.MatchString("^(?:"+field.Validation+")$", s)
				if err != nil {
					errs[field.Name] = fmt.Sprintf("invalid validation pattern: %v", err)
				} else if !matched {
					errs[field.Name] = fmt.Sprintf("does not match pattern %q", field.Validation)
				}
			}
		case "number":
			if _, ok := toFloat(val); !ok {
				errs[field.Name] = "must be a number"
			}
		case "boolean":
			if _, ok := toBool(val); !ok {
				errs[field.Name] = "must be a boolean"
			}
		case "enum":
			s, ok := toString(val)
			if !ok {
				errs[field.Name] = "must be a string"
				continue
			}
			if !contains(field.Options, s) {
				errs[field.Name] = fmt.Sprintf("must be one of: %s", strings.Join(field.Options, ", "))
			}
		}
	}
	return errs
}

func isEmpty(v interface{}) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		return s == ""
	}
	return false
}

func toString(v interface{}) (string, bool) {
	switch s := v.(type) {
	case string:
		return s, true
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64), true
	case int:
		return strconv.Itoa(s), true
	case bool:
		return strconv.FormatBool(s), true
	}
	return "", false
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

func toBool(v interface{}) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		bv, err := strconv.ParseBool(b)
		return bv, err == nil
	}
	return false, false
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
