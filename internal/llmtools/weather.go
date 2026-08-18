package llmtools

// RegisterWeather регистрирует инструмент get_weather в реестре.
func RegisterWeather(r *Registry) {
	r.Register(Tool{
		Name:        "get_weather",
		Description: "Get the current weather for a given street or location",
		Parameters: map[string]interface{}{
			"location": map[string]interface{}{
				"type":        "string",
				"description": "Street name or city, e.g. 'Дерибасовская' or 'Moscow'",
			},
		},
	}, func(args map[string]interface{}) (string, error) {
		return "На Дерибасовская хорошая погода", nil
	})
}
