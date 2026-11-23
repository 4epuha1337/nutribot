package nutrition

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"nutribot/types"
)

const nutrientEstimatePrompt = `Оцени пищевую ценность продукта на 100 грамм (или 100 мл для жидкостей), опираясь на типичные справочные данные.

Верни JSON-объект строго такой структуры:
{"calories": 165, "protein": 31, "fat": 3.6, "carbs": 0}

Продукт: %s`

var nutrientEstimateSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"calories": map[string]interface{}{"type": "number"},
		"protein":  map[string]interface{}{"type": "number"},
		"fat":      map[string]interface{}{"type": "number"},
		"carbs":    map[string]interface{}{"type": "number"},
	},
	"required":             []string{"calories", "protein", "fat", "carbs"},
	"additionalProperties": false,
}

func (c *groqClient) EstimateNutrients(ctx context.Context, productName string) (*types.Nutrients, error) {
	nutrients, err := c.estimateOnce(ctx, productName)
	if err != nil {
		nutrients, err = c.estimateOnce(ctx, productName) // одна повторная попытка
		if err != nil {
			return nil, fmt.Errorf("estimate failed after retry: %w", err)
		}
	}
	return nutrients, nil
}

func (c *groqClient) estimateOnce(ctx context.Context, productName string) (*types.Nutrients, error) {
	reqBody := types.ChatRequest{
		Model: c.model,
		Messages: []types.ChatMessage{
			{Role: "user", Content: fmt.Sprintf(nutrientEstimatePrompt, productName)},
		},
		Temperature: 0.1,
		ResponseFormat: types.ResponseFormat{
			Type: "json_schema",
			JSONSchema: types.JsonSchema{
				Name:   "nutrient_estimate",
				Strict: true,
				Schema: nutrientEstimateSchema,
			},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://api.groq.com/openai/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp types.ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}
	if apiResp.Error != nil {
		return nil, fmt.Errorf("groq api error: %s", apiResp.Error.Message)
	}
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from groq")
	}

	var n types.Nutrients
	content := apiResp.Choices[0].Message.Content
	if err := json.Unmarshal([]byte(content), &n); err != nil {
		return nil, fmt.Errorf("failed to parse nutrients JSON: %w (raw: %s)", err, content)
	}

	return &n, nil
}
