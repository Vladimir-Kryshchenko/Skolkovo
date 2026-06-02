package navindex

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// navNamespace — фиксированный namespace для детерминированных UUID узлов
// (один и тот же узел при переиндексации получает тот же ID, без дублей в Qdrant).
var navNamespace = uuid.NewSHA1(uuid.Nil, []byte("baza-skolkovo/navindex"))

// nodeID возвращает детерминированный UUID узла по ключу «порт|маршрут|блок».
func nodeID(port, route, block string) string {
	return uuid.NewSHA1(navNamespace, []byte(port+"|"+route+"|"+block)).String()
}

// Flatten разворачивает дерево в плоский список узлов для индексации/поиска:
// по одному узлу на страницу (обзор) и по одному на каждый блок страницы.
func Flatten(tree []Interface) []Node {
	var nodes []Node
	for _, iface := range tree {
		for _, p := range iface.Pages {
			// Узел-обзор страницы.
			blockNames := make([]string, 0, len(p.Blocks))
			for _, b := range p.Blocks {
				blockNames = append(blockNames, b.Name)
			}
			pageText := fmt.Sprintf(
				"Интерфейс «%s» (порт %s, для: %s). Страница «%s» — маршрут %s. Назначение: %s. Как попасть: %s Блоки на странице: %s.",
				iface.Name, iface.Port, iface.Audience, p.Title, p.Route, p.Purpose, p.HowTo, strings.Join(blockNames, "; "),
			)
			nodes = append(nodes, Node{
				ID:        nodeID(iface.Port, p.Route, ""),
				Interface: iface.Name, Port: iface.Port, Audience: iface.Audience,
				Route: p.Route, PageTitle: p.Title, HowTo: p.HowTo, Kind: "page", Text: pageText,
			})
			// Узлы блоков.
			for _, b := range p.Blocks {
				var sb strings.Builder
				fmt.Fprintf(&sb, "Интерфейс «%s» (порт %s). Страница «%s» (%s). Блок «%s» (%s).",
					iface.Name, iface.Port, p.Title, p.Route, b.Name, b.Kind)
				if b.Purpose != "" {
					fmt.Fprintf(&sb, " Назначение: %s.", b.Purpose)
				}
				if len(b.Labels) > 0 {
					fmt.Fprintf(&sb, " Элементы: %s.", strings.Join(b.Labels, ", "))
				}
				fmt.Fprintf(&sb, " Как попасть: %s", p.HowTo)
				nodes = append(nodes, Node{
					ID:        nodeID(iface.Port, p.Route, b.Name),
					Interface: iface.Name, Port: iface.Port, Audience: iface.Audience,
					Route: p.Route, PageTitle: p.Title, HowTo: p.HowTo,
					Block: b.Name, Kind: b.Kind, Text: sb.String(),
				})
			}
		}
	}
	return nodes
}

// ToJSON сериализует дерево навигации (источник истины) в отступленный JSON.
func ToJSON(tree []Interface) ([]byte, error) {
	return json.MarshalIndent(tree, "", "  ")
}

// ToMarkdown рендерит человекочитаемую навигационную карту для RAG-каталога.
func ToMarkdown(tree []Interface) string {
	var sb strings.Builder
	sb.WriteString("# RAG: Полная структура сайта «База Сколково»\n\n")
	sb.WriteString("> Навигационная карта для ИИ-чат-бота: каждая страница, вкладка, блок и надпись.\n")
	sb.WriteString("> Сгенерировано из исходного кода пакетом `src/navindex` (источник истины — `Tree()`).\n")
	sb.WriteString("> Не редактируйте вручную: при изменении страниц правьте `src/navindex/tree.go` и пересоберите командой `navindex`.\n\n")

	sb.WriteString("## Карта интерфейсов\n\n")
	sb.WriteString("| Порт | Интерфейс | Для кого | Как попасть |\n| :--- | :--- | :--- | :--- |\n")
	for _, iface := range tree {
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", iface.Port, iface.Name, iface.Audience, iface.HowTo))
	}
	sb.WriteString("\n---\n\n")

	for _, iface := range tree {
		sb.WriteString(fmt.Sprintf("## %s (%s)\n\n", iface.Name, iface.Port))
		sb.WriteString(fmt.Sprintf("- **Аудитория:** %s\n- **Аутентификация:** %s\n- **Как попасть:** %s\n\n", iface.Audience, iface.Auth, iface.HowTo))
		for _, p := range iface.Pages {
			sb.WriteString(fmt.Sprintf("### %s — `%s`\n\n", p.Title, p.Route))
			sb.WriteString(fmt.Sprintf("**Назначение:** %s  \n**Как попасть:** %s\n\n", p.Purpose, p.HowTo))
			for _, b := range p.Blocks {
				sb.WriteString(fmt.Sprintf("- **%s** _(%s)_", b.Name, b.Kind))
				if b.Purpose != "" {
					sb.WriteString(" — " + b.Purpose)
				}
				sb.WriteString("\n")
				if len(b.Labels) > 0 {
					sb.WriteString("  - Элементы: " + strings.Join(b.Labels, "; ") + "\n")
				}
			}
			sb.WriteString("\n")
		}
		sb.WriteString("---\n\n")
	}
	return sb.String()
}
