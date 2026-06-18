package anthropicadapter

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Hieun003/sonar"
)

// ErrParseToolInput is returned when the tool input cannot be parsed.
var ErrParseToolInput = errors.New("anthropicadapter: failed to parse tool input")

// GetToolDefinition returns the Anthropic Tool definition for the elicitation tool.
// This definition tells Anthropic Claude models how to call the elicit_user_input function.
//
// Usage:
//
//	toolDef := anthropicadapter.GetToolDefinition()
func GetToolDefinition() map[string]any {
	return map[string]any{
		"name":        "elicit_user_input",
		"description": "Ask the human user a set of questions and wait for their response.",
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"questions": map[string]any{
					"type":        "array",
					"description": "List of questions to present to the user.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"question": map[string]any{
								"type":        "string",
								"description": "The question text to display.",
							},
							"type": map[string]any{
								"type":        "string",
								"enum":        []any{"single_select", "multi_select", "rank_priorities"},
								"description": "The interaction type.",
							},
							"options": map[string]any{
								"type":        "array",
								"items":       map[string]any{"type": "string"},
								"description": "List of options for the user to choose from.",
							},
						},
						"required": []any{"question", "type", "options"},
					},
				},
			},
			"required": []any{"questions"},
		},
	}
}

// toolInput represents the raw JSON structure of Claude's tool input block.
type toolInput struct {
	Questions []elicit.Question `json:"questions"`
}

// ParseToolInput parses the JSON string containing tool input returned by Claude
// and returns a slice of elicit.Question.
// It wraps JSON decoding errors with ErrParseToolInput and returns a plain error if no questions are found.
//
// Usage:
//
//	questions, err := anthropicadapter.ParseToolInput(inputJSON)
func ParseToolInput(inputJSON string) ([]elicit.Question, error) {
	var in toolInput
	if err := json.Unmarshal([]byte(inputJSON), &in); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParseToolInput, err)
	}
	if len(in.Questions) == 0 {
		return nil, errors.New("anthropicadapter: tool input contains no questions")
	}
	return in.Questions, nil
}
