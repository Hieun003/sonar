package openaiadapter

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Hieun003/sonar"
)

// GetToolDefinition returns the OpenAI Tool definition for the elicitation tool.
// This definition tells OpenAI LLM models how to call the elicit_user_input function.
//
// Usage:
//
//	toolDef := openaiadapter.GetToolDefinition()
func GetToolDefinition() map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "elicit_user_input",
			"description": "Ask the human user a set of questions and wait for their response.",
			"parameters": map[string]any{
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
		},
	}
}

// toolArguments represents the raw JSON structure of the LLM's arguments response.
type toolArguments struct {
	Questions []elicit.Question `json:"questions"`
}

// ErrParseToolArguments is returned when the tool arguments cannot be parsed.
var ErrParseToolArguments = errors.New("openaiadapter: failed to parse tool arguments")

// ErrNoQuestions is returned when the tool arguments do not contain any questions.
var ErrNoQuestions = errors.New("openaiadapter: tool arguments contain no questions")

// ParseToolArguments parses the JSON string containing tool arguments returned by the LLM
// and returns a slice of elicit.Question.
// It wraps JSON decoding errors with ErrParseToolArguments and returns ErrNoQuestions if no questions are found.
//
// Usage:
//
//	questions, err := openaiadapter.ParseToolArguments(argsJSON)
func ParseToolArguments(argsJSON string) ([]elicit.Question, error) {
	var args toolArguments
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParseToolArguments, err)
	}
	if len(args.Questions) == 0 {
		return nil, ErrNoQuestions
	}
	return args.Questions, nil
}
