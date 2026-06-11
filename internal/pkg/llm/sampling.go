package llm

import (
	"github.com/cloudwego/eino/components/model"
	openai "github.com/sashabaranov/go-openai"
)

// SamplingParams holds optional LLM generation parameters. Fields left nil
// are omitted from upstream API requests so the provider's defaults apply.
type SamplingParams struct {
	Temperature      *float32
	TopP             *float32
	MaxTokens        *int
	FrequencyPenalty *float32
	PresencePenalty  *float32
}

// PtrFloat32 returns a pointer to v. Useful for inline SamplingParams literals.
func PtrFloat32(v float32) *float32 { return &v }

// PtrInt returns a pointer to v.
func PtrInt(v int) *int { return &v }

// EinoOptions converts set fields into eino model.Option values for the
// graph ChatModel path. Unset fields add no option.
func (s SamplingParams) EinoOptions() []model.Option {
	var opts []model.Option
	if s.Temperature != nil {
		opts = append(opts, model.WithTemperature(*s.Temperature))
	}
	if s.TopP != nil {
		opts = append(opts, model.WithTopP(*s.TopP))
	}
	if s.MaxTokens != nil {
		opts = append(opts, model.WithMaxTokens(*s.MaxTokens))
	}
	return opts
}

// applyToOpenAIReq copies only the set fields onto the SDK request.
func (s SamplingParams) applyToOpenAIReq(req *openai.ChatCompletionRequest) {
	if s.Temperature != nil {
		req.Temperature = *s.Temperature
	}
	if s.TopP != nil {
		req.TopP = *s.TopP
	}
	if s.MaxTokens != nil {
		req.MaxTokens = *s.MaxTokens
	}
	if s.FrequencyPenalty != nil {
		req.FrequencyPenalty = *s.FrequencyPenalty
	}
	if s.PresencePenalty != nil {
		req.PresencePenalty = *s.PresencePenalty
	}
}

// fromEinoOptions extracts the common eino model options the adapter
// understands into our SamplingParams bag.
func fromEinoOptions(common *model.Options) SamplingParams {
	if common == nil {
		return SamplingParams{}
	}
	return SamplingParams{
		Temperature: common.Temperature,
		TopP:        common.TopP,
		MaxTokens:   common.MaxTokens,
	}
}
