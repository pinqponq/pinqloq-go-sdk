package pinqloq

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestApplyBodyRedactionBuiltInFloor(t *testing.T) {
	plan := NewRedactionPlan(false, nil)
	body := `{"email":"a@b.com","password":"hunter2"}`

	result := ApplyBodyRedaction(body, plan)

	var parsed map[string]string
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["password"] != redactedValue {
		t.Fatalf("expected password redacted, got %q", parsed["password"])
	}
	if parsed["email"] != "a@b.com" {
		t.Fatalf("expected email untouched, got %q", parsed["email"])
	}
}

func TestApplyBodyRedactionNestedAndArrays(t *testing.T) {
	plan := NewRedactionPlan(false, []string{"token"})
	body := `{"users":[{"token":"abc"},{"token":"def"}],"nested":{"token":"ghi"}}`

	result := ApplyBodyRedaction(body, plan)
	if strings.Contains(result, "abc") || strings.Contains(result, "def") || strings.Contains(result, "ghi") {
		t.Fatalf("expected all tokens redacted, got %s", result)
	}
}

func TestApplyBodyRedactionDoesNotMatchPartialWord(t *testing.T) {
	plan := NewRedactionPlan(false, nil)
	body := `{"shippingAddress":"123 Main St"}`

	if got := ApplyBodyRedaction(body, plan); got != body {
		t.Fatalf("expected body unchanged, got %s", got)
	}
}

func TestApplyBodyRedactionFailsClosedOnUnparseableSensitiveBody(t *testing.T) {
	plan := NewRedactionPlan(false, nil)
	body := "email=a%40b.com&password=hunter2"

	result := ApplyBodyRedaction(body, plan)
	if !strings.Contains(result, "REDACTED: body carries a credential field") {
		t.Fatalf("expected fail-closed message, got %s", result)
	}
}

func TestApplyBodyRedactionDetectsCamelCaseCredential(t *testing.T) {
	plan := NewRedactionPlan(false, nil)
	unparseable := "authToken=abc123&userId=64"

	result := ApplyBodyRedaction(unparseable, plan)
	if !strings.Contains(result, "REDACTED: body carries a credential field") {
		t.Fatalf("expected camelCase hump detection to trigger fail-closed, got %s", result)
	}
}

func TestApplyBodyRedactionReturnsNonSensitiveUnparseableBodyUnchanged(t *testing.T) {
	plan := NewRedactionPlan(false, nil)
	body := "plain text response"

	if got := ApplyBodyRedaction(body, plan); got != body {
		t.Fatalf("expected body unchanged, got %s", got)
	}
}

func TestApplyBodyRedactionRedactAllMasksEveryLeaf(t *testing.T) {
	body := `{"cardNumber":"4111","amount":100}`

	result := ApplyBodyRedaction(body, redactionPlanAll)
	var parsed map[string]string
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["cardNumber"] != redactedValue || parsed["amount"] != redactedValue {
		t.Fatalf("expected every leaf redacted, got %s", result)
	}
}

func TestSerializeHeadersRedactsBuiltInFloor(t *testing.T) {
	plan := NewRedactionPlan(false, nil)
	headers := http.Header{}
	headers.Set("Authorization", "Bearer xyz")
	headers.Set("X-Request-Id", "r1")

	result := SerializeHeaders(headers, plan)
	var parsed map[string]string
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["Authorization"] != redactedValue {
		t.Fatalf("expected Authorization redacted, got %q", parsed["Authorization"])
	}
	if parsed["X-Request-Id"] != "r1" {
		t.Fatalf("expected X-Request-Id untouched, got %q", parsed["X-Request-Id"])
	}
}
