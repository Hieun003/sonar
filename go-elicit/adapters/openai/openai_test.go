package openaiadapter

import (
	"errors"
	"testing"

	"github.com/Hieun003/sonar"
)

func TestParseToolArgumentsValid(t *testing.T) {
	validJSON := `{
		"questions": [
			{
				"question": "What is your favorite color?",
				"type": "single_select",
				"options": ["Red", "Green", "Blue"]
			}
		]
	}`

	questions, err := ParseToolArguments(validJSON)
	if err != nil {
		t.Fatalf("unexpected error parsing valid JSON: %v", err)
	}

	if len(questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(questions))
	}

	q := questions[0]
	if q.Question != "What is your favorite color?" {
		t.Errorf("unexpected question text: %s", q.Question)
	}
	if q.Type != elicit.SingleSelect {
		t.Errorf("unexpected question type: %s", q.Type)
	}
	if len(q.Options) != 3 || q.Options[0] != "Red" {
		t.Errorf("unexpected options: %v", q.Options)
	}
}

func TestParseToolArgumentsInvalidJSON(t *testing.T) {
	invalidJSON := `{"questions": [`

	_, err := ParseToolArguments(invalidJSON)
	if err == nil {
		t.Fatal("expected error parsing invalid JSON, got nil")
	}

	if !errors.Is(err, ErrParseToolArguments) {
		t.Errorf("expected error to be ErrParseToolArguments, got %v", err)
	}
}

func TestParseToolArgumentsEmptyQuestions(t *testing.T) {
	emptyJSON := `{"questions": []}`

	_, err := ParseToolArguments(emptyJSON)
	if err == nil {
		t.Fatal("expected error parsing empty questions, got nil")
	}

	if !errors.Is(err, ErrNoQuestions) {
		t.Errorf("expected error ErrNoQuestions, got %v", err)
	}
}

func TestGetToolDefinition(t *testing.T) {
	def := GetToolDefinition()

	if def["type"] != "function" {
		t.Errorf("expected type to be 'function', got %v", def["type"])
	}

	fn, ok := def["function"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'function' to be a map[string]any")
	}

	if fn["name"] != "elicit_user_input" {
		t.Errorf("expected function name to be 'elicit_user_input', got %v", fn["name"])
	}

	params, ok := fn["parameters"].(map[string]any)
	if !ok {
		t.Fatal("expected 'parameters' to be a map[string]any")
	}

	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected 'properties' to be a map[string]any")
	}

	if _, ok := props["questions"]; !ok {
		t.Error("expected 'questions' property to exist")
	}
}
