package anthropicadapter

import (
	"errors"
	"testing"

	"github.com/Hieun003/sonar"
)

func TestParseToolInputValid(t *testing.T) {
	validJSON := `{
		"questions": [
			{
				"question": "What is your favorite color?",
				"type": "single_select",
				"options": ["Red", "Green", "Blue"]
			}
		]
	}`

	questions, err := ParseToolInput(validJSON)
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

func TestParseToolInputInvalidJSON(t *testing.T) {
	invalidJSON := `{"questions": [`

	_, err := ParseToolInput(invalidJSON)
	if err == nil {
		t.Fatal("expected error parsing invalid JSON, got nil")
	}

	if !errors.Is(err, ErrParseToolInput) {
		t.Errorf("expected error to be ErrParseToolInput, got %v", err)
	}
}

func TestParseToolInputEmptyQuestions(t *testing.T) {
	emptyJSON := `{"questions": []}`

	_, err := ParseToolInput(emptyJSON)
	if err == nil {
		t.Fatal("expected error parsing empty questions, got nil")
	}

	expectedErr := "anthropicadapter: tool input contains no questions"
	if err.Error() != expectedErr {
		t.Errorf("expected error %q, got %q", expectedErr, err.Error())
	}
}

func TestGetToolDefinition(t *testing.T) {
	def := GetToolDefinition()

	if def["name"] != "elicit_user_input" {
		t.Errorf("expected name to be 'elicit_user_input', got %v", def["name"])
	}

	schema, ok := def["input_schema"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'input_schema' to be a map[string]any")
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected 'properties' to be a map[string]any")
	}

	if _, ok := props["questions"]; !ok {
		t.Error("expected 'questions' property to exist")
	}
}
