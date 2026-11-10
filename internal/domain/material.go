package domain

type Material struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Unit     string `json:"unit"`
}

type MaterialInput struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Unit     string `json:"unit"`
}

type MaterialPatch struct {
	Name     *string `json:"name"`
	Category *string `json:"category"`
	Unit     *string `json:"unit"`
}

type MaterialFilter struct {
	Search, Category, Sort, Order string
	Page                          Page
}
