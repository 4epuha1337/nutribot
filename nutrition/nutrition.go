package nutrition

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"nutribot/types"
)

const extractionPrompt = `Ты извлекаешь ингредиенты приёма пищи из текста пользователя и возвращаешь их в формате JSON.

Верни JSON-объект строго такой структуры:
{"ingredients": [{"name": "куриная грудка", "amount": 150, "unit": "г"}]}

Правила:
- Если количество не указано явно, оцени разумный размер порции.
- amount указывай числом в граммах или миллилитрах.
- Название ингредиента — в базовой форме, без бренда.
- Если в тексте нет еды, верни {"ingredients": []}.

Текст: %s`

type groqClient struct {
	apiKey string
	model  string
	http   *http.Client
}

func NewGroqClient(apiKey string) *groqClient {
	return &groqClient{apiKey: apiKey, model: "openai/gpt-oss-120b", http: &http.Client{}}
}

var ingredientsSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"ingredients": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":   map[string]interface{}{"type": "string"},
					"amount": map[string]interface{}{"type": "number"},
					"unit":   map[string]interface{}{"type": "string"},
				},
				"required":             []string{"name", "amount", "unit"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"ingredients"},
	"additionalProperties": false,
}

func (c *groqClient) ExtractIngredients(ctx context.Context, text string) ([]types.Ingredient, error) {
	ingredients, err := c.extractOnce(ctx, text)
	if err != nil {
		ingredients, err = c.extractOnce(ctx, text) // одна повторная попытка
		if err != nil {
			return nil, fmt.Errorf("extraction failed after retry: %w", err)
		}
	}
	return ingredients, nil
}

func (c *groqClient) extractOnce(ctx context.Context, text string) ([]types.Ingredient, error) {
	reqBody := types.ChatRequest{
		Model: c.model,
		Messages: []types.ChatMessage{
			{Role: "user", Content: fmt.Sprintf(extractionPrompt, text)},
		},
		Temperature: 0.2,
		ResponseFormat: types.ResponseFormat{
			Type: "json_schema",
			JSONSchema: types.JsonSchema{
				Name:   "ingredients_extraction",
				Strict: true,
				Schema: ingredientsSchema,
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

	var wrapper types.IngredientsWrapper
	content := apiResp.Choices[0].Message.Content
	if err := json.Unmarshal([]byte(content), &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w (raw: %s)", err, content)
	}

	return wrapper.Ingredients, nil
}
