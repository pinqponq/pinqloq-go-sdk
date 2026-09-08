package pinqloq

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode"
)

const redactedValue = "*****REDACTED*****"

const unparseableSensitiveBodyValue = "*****REDACTED: body carries a credential field and is not parseable JSON " +
	"(non-JSON content type, or longer than the capture limit)*****"

const malformedBodyWarningThrottle = 60 * time.Second

var alwaysRedactedNames = buildNameSet([]string{
	"authorization",
	"proxy-authorization",
	"cookie",
	"set-cookie",
	"x-api-key",
	"x-secret-key",
	"x-auth-token",
	"x-access-token",
	"x-csrf-token",
	"x-xsrf-token",
	"secret_key",
	"password",
	"newpassword",
	"oldpassword",
	"currentpassword",
	"passwordconfirmation",
	"confirmpassword",
	"secret",
	"secretkey",
	"clientsecret",
	"apikey",
	"accesstoken",
	"refreshtoken",
	"idtoken",
	"token",
	"otp",
	"otpcode",
	"verificationcode",
	"pin",
	"privatekey",
	"cardnumber",
	"cvv",
	"cvc",
	"securitycode",
	"iban",
	"ssn",
})

func buildNameSet(names []string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[strings.ToLower(name)] = struct{}{}
	}
	return set
}

func isLetterOrDigit(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func boundedNameMatch(original []rune, index, length int) bool {
	end := index + length

	var before, after rune
	hasBefore := index > 0
	if hasBefore {
		before = original[index-1]
	}
	at := original[index]
	hasAfter := end < len(original)
	if hasAfter {
		after = original[end]
	}

	startBounded := !hasBefore || !isLetterOrDigit(before) || (unicode.IsUpper(at) && !unicode.IsUpper(before))
	endBounded := !hasAfter || !isLetterOrDigit(after) || unicode.IsUpper(after)

	return startBounded && endBounded
}

func containsWholeName(body string, names map[string]struct{}) bool {
	if len(names) == 0 {
		return false
	}

	original := []rune(body)
	lower := []rune(strings.ToLower(body))

	for name := range names {
		nameRunes := []rune(name)
		n := len(nameRunes)
		if n == 0 || n > len(lower) {
			continue
		}

		for i := 0; i+n <= len(lower); i++ {
			if runeSliceEqual(lower[i:i+n], nameRunes) && boundedNameMatch(original, i, n) {
				return true
			}
		}
	}

	return false
}

func runeSliceEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// RedactionPlan is the redaction decision for a request. RedactAll masks every field/header
// value wholesale (the RedactPaths equivalent of the .NET SDK's [PinqloqRedactEndpoint]);
// otherwise ShouldRedact is true for names configured via RedactFields plus the always-redacted
// floor.
type RedactionPlan struct {
	RedactAll     bool
	declaredNames map[string]struct{}
}

// NewRedactionPlan builds a plan that redacts the given declared field/header names (case
// insensitive) in addition to the built-in credential-name floor.
func NewRedactionPlan(redactAll bool, declaredNames []string) *RedactionPlan {
	return &RedactionPlan{RedactAll: redactAll, declaredNames: buildNameSet(declaredNames)}
}

var redactionPlanNone = NewRedactionPlan(false, nil)
var redactionPlanAll = NewRedactionPlan(true, nil)

// HasDeclaredRedactions reports whether this plan redacts anything beyond the built-in floor.
func (p *RedactionPlan) HasDeclaredRedactions() bool {
	return p.RedactAll || len(p.declaredNames) > 0
}

// ShouldRedact reports whether the given JSON field or header name needs to be redacted.
func (p *RedactionPlan) ShouldRedact(name string) bool {
	lower := strings.ToLower(name)
	if p.RedactAll {
		return true
	}
	if _, ok := alwaysRedactedNames[lower]; ok {
		return true
	}
	_, ok := p.declaredNames[lower]
	return ok
}

// ContainsDeclaredName reports whether a raw body mentions one of this plan's declared names, as
// a whole name.
func (p *RedactionPlan) ContainsDeclaredName(body string) bool {
	return containsWholeName(body, p.declaredNames)
}

// ContainsAlwaysRedactedName reports whether a raw body mentions any always-redacted name, as a
// whole name.
func ContainsAlwaysRedactedName(body string) bool {
	return containsWholeName(body, alwaysRedactedNames)
}

func redactValue(value any, plan *RedactionPlan) any {
	switch v := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, item := range v {
			if plan.ShouldRedact(key) {
				result[key] = redactedValue
			} else {
				result[key] = redactValue(item, plan)
			}
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = redactValue(item, plan)
		}
		return result
	default:
		return value
	}
}

func redactFully(value any) any {
	switch v := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, item := range v {
			result[key] = redactFully(item)
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = redactFully(item)
		}
		return result
	default:
		return redactedValue
	}
}

func redactJSONProperties(body string, plan *RedactionPlan, sensitive bool) string {
	if strings.TrimSpace(body) == "" {
		return body
	}

	var parsed any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		warnThrottled(
			"redactJSONProperties",
			malformedBodyWarningThrottle,
			"pinqloq: a captured body could not be parsed as JSON; falling back to whole-body handling. %v",
			err,
		)
		if sensitive {
			return unparseableSensitiveBodyValue
		}
		return body
	}

	redacted, err := json.Marshal(redactValue(parsed, plan))
	if err != nil {
		return body
	}
	return string(redacted)
}

func redactJSONFully(body string) string {
	if strings.TrimSpace(body) == "" {
		return body
	}

	var parsed any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		warnThrottled(
			"redactJSONFully",
			malformedBodyWarningThrottle,
			"pinqloq: a captured body under a RedactAll plan could not be parsed as JSON; masking it wholesale. %v",
			err,
		)
		return redactedValue
	}

	redacted, err := json.Marshal(redactFully(parsed))
	if err != nil {
		return redactedValue
	}
	return string(redacted)
}

// ApplyBodyRedaction processes a request/response body according to the redaction plan. If
// plan.RedactAll, every JSON value is redacted; otherwise only fields the plan names (plus the
// built-in floor) are redacted. A body that mentions nothing sensitive is returned unprocessed.
func ApplyBodyRedaction(body string, plan *RedactionPlan) string {
	if plan.RedactAll {
		return redactJSONFully(body)
	}

	mentionsCredential := ContainsAlwaysRedactedName(body) || plan.ContainsDeclaredName(body)
	if !plan.HasDeclaredRedactions() && !mentionsCredential {
		return body
	}

	return redactJSONProperties(body, plan, mentionsCredential)
}

// SerializeHeaders converts an http.Header into single-line JSON, redacting values whose name
// matches the plan. Multi-value headers are joined with ", ".
func SerializeHeaders(headers http.Header, plan *RedactionPlan) string {
	result := make(map[string]string, len(headers))
	for key, values := range headers {
		if plan.ShouldRedact(key) {
			result[key] = redactedValue
		} else {
			result[key] = strings.Join(values, ", ")
		}
	}

	data, err := json.Marshal(result)
	if err != nil {
		return "{}"
	}
	return string(data)
}
