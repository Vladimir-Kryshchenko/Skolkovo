// Package navindex — навигационная карта всех веб-интерфейсов «База Сколково».
//
// Это специализированная RAG-база структуры сайта: каждая страница, вкладка,
// блок и ключевая надпись описаны как данные (источник истины — Tree()).
// Из него генерируются: JSON (navigation.json), человекочитаемая Markdown-карта
// и векторный индекс в отдельной Qdrant-коллекции для инструмента get_navigation.
//
// Назначение — дать ИИ-чат-боту точную навигацию: «где это находится на сайте»
// и «как туда попасть», не смешивая с семантическим поиском по документам.
package navindex

// Interface — отдельный веб-интерфейс системы (один порт, одна аудитория).
type Interface struct {
	Port     string `json:"port"`
	Name     string `json:"name"`
	Audience string `json:"audience"`
	Auth     string `json:"auth"`
	HowTo    string `json:"howto"` // как попасть в интерфейс
	Pages    []Page `json:"pages"`
}

// Page — страница интерфейса (HTTP-маршрут с UI).
type Page struct {
	Route   string  `json:"route"`
	Title   string  `json:"title"`
	Purpose string  `json:"purpose"`
	HowTo   string  `json:"howto"` // как открыть страницу (меню → пункт)
	Blocks  []Block `json:"blocks"`
}

// Block — вкладка, секция, форма, таблица или группа элементов на странице.
type Block struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"` // tab | section | form | table | nav | button | widget
	Purpose string   `json:"purpose,omitempty"`
	Labels  []string `json:"labels,omitempty"` // надписи, поля, кнопки, колонки
}

// Node — плоский узел навигации для индексации/поиска (страница или блок).
type Node struct {
	ID        string `json:"id"`
	Interface string `json:"interface"`
	Port      string `json:"port"`
	Audience  string `json:"audience"`
	Route     string `json:"route"`
	PageTitle string `json:"page_title"`
	HowTo     string `json:"howto"`
	Block     string `json:"block,omitempty"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
}
