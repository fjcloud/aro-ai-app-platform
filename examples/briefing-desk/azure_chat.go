package main

import (
	"encoding/json"
	"errors"
)

// The workshop's DeepSeek deployment supports reasoning_effort=none through
// Chat Completions. Azure's Responses endpoint rejects that setting. Translate
// the wire format only; the common Go loop still validates and executes tools.
func azureChatRequest(body []byte) ([]byte, error) {
	var in struct {
		Model        string           `json:"model"`
		Instructions string           `json:"instructions"`
		Input        []map[string]any `json:"input"`
		Tools        []map[string]any `json:"tools"`
		Text         map[string]any   `json:"text"`
		ToolChoice   string           `json:"tool_choice"`
		MaxTokens    int              `json:"max_output_tokens"`
	}
	if json.Unmarshal(body, &in) != nil {
		return nil, errors.New("Invalid Azure research request")
	}
	messages := []any{map[string]any{"role": "system", "content": in.Instructions}}
	for _, item := range in.Input {
		switch item["type"] {
		case "function_call":
			messages = append(messages, map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": item["call_id"], "type": "function", "function": map[string]any{"name": item["name"], "arguments": item["arguments"]}}}})
		case "function_call_output":
			messages = append(messages, map[string]any{"role": "tool", "tool_call_id": item["call_id"], "content": item["output"]})
		default:
			messages = append(messages, map[string]any{"role": item["role"], "content": item["content"]})
		}
	}
	out := map[string]any{"model": in.Model, "messages": messages, "max_completion_tokens": in.MaxTokens, "reasoning_effort": "none", "temperature": 0.1, "stream": false, "store": false}
	if len(in.Tools) > 0 {
		tools := []any{}
		for _, spec := range in.Tools {
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": spec["name"], "description": spec["description"], "parameters": spec["parameters"]}})
		}
		out["tools"] = tools
		out["tool_choice"] = in.ToolChoice
		out["parallel_tool_calls"] = false
	} else if f, ok := in.Text["format"].(map[string]any); ok {
		out["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": f["name"], "strict": f["strict"], "schema": f["schema"]}}
	}
	return json.Marshal(out)
}

func azureChatResponse(raw []byte) ([]byte, error) {
	var in struct {
		Model   string `json:"model"`
		Choices []struct {
			Finish  string `json:"finish_reason"`
			Message struct {
				Content string `json:"content"`
				Calls   []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &in) != nil || len(in.Choices) != 1 {
		return nil, errors.New("Invalid Azure DeepSeek response")
	}
	choice := in.Choices[0]
	if choice.Finish != "stop" && choice.Finish != "tool_calls" {
		return nil, errors.New("Azure DeepSeek response was truncated or did not complete")
	}
	output := []any{}
	for _, call := range choice.Message.Calls {
		if call.Type != "function" {
			return nil, errors.New("Azure requested an unsupported tool type")
		}
		output = append(output, map[string]any{"type": "function_call", "call_id": call.ID, "name": call.Function.Name, "arguments": call.Function.Arguments})
	}
	if len(output) == 0 {
		output = append(output, map[string]any{"type": "message", "content": []any{map[string]string{"type": "output_text", "text": choice.Message.Content}}})
	}
	return json.Marshal(map[string]any{"status": "completed", "model": in.Model, "output": output})
}
