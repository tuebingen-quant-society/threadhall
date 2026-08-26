package codex

import (
	"encoding/json"
	"strings"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
)

func captureQuestions(raw json.RawMessage) ([]agenttask.Question, bool) {
	var params struct {
		Questions []struct {
			ID, Header, Question string
			IsOther, IsSecret    bool
			Options              []agenttask.QuestionOption
		} `json:"questions"`
	}
	if json.Unmarshal(raw, &params) != nil || len(params.Questions) == 0 {
		return nil, false
	}
	questions := make([]agenttask.Question, 0, len(params.Questions))
	for _, item := range params.Questions {
		if item.IsSecret {
			return nil, false
		}
		questions = append(questions, agenttask.Question{
			ID: item.ID, Header: item.Header, Question: item.Question, IsOther: item.IsOther, Options: item.Options,
		})
	}
	return questions, agenttask.ValidQuestions(questions)
}

func questionOutput(questions []agenttask.Question) string {
	var output strings.Builder
	output.WriteString("I need your input before I continue.\n")
	for _, question := range questions {
		output.WriteString("\n**" + question.Header + ":** " + question.Question + "\n")
		for _, option := range question.Options {
			output.WriteString("- " + option.Label)
			if option.Description != "" {
				output.WriteString(" — " + option.Description)
			}
			output.WriteByte('\n')
		}
	}
	return output.String()
}
