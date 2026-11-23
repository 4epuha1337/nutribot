package types

type TimeEntry struct {
	Hour   int `json:"hour"`
	Minute int `json:"minute"`
}

type User struct {
	Id     int         `json:"id"`
	ChatID int         `json:"chatid"`
	Offset int         `json:"offset"`
	Time   []TimeEntry `json:"time"`
	Name   string      `json:"name"`
	Age    int         `json:"age"`
	State  int
}

type ReminderWithID struct {
	ID       int64
	Reminder TimeEntry
}

type Ingredient struct {
	Name   string  `json:"name"`
	Amount float64 `json:"amount"`
	Unit   string  `json:"unit"`
}

type Nutrients struct {
	Calories float64 `json:"calories"`
	Protein  float64 `json:"protein"`
	Fat      float64 `json:"fat"`
	Carbs    float64 `json:"carbs"`
}

type IngredientMatch struct {
	Requested string
	Matched   string
	Score     int
	Source    string
}

type MealResult struct {
	Ingredients []Ingredient
	Total       Nutrients
	NotFound    []string
	Matches     []IngredientMatch
}

type ResponseFormat struct {
	Type       string     `json:"type"`
	JSONSchema JsonSchema `json:"json_schema"`
}

type IngredientsWrapper struct {
	Ingredients []Ingredient `json:"ingredients"`
}

type JsonSchema struct {
	Name   string      `json:"name"`
	Strict bool        `json:"strict"`
	Schema interface{} `json:"schema"`
}

type ChatRequest struct {
	Model          string         `json:"model"`
	Temperature    float64        `json:"temperature"`
	Messages       []ChatMessage  `json:"messages"`
	ResponseFormat ResponseFormat `json:"response_format"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}
