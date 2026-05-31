// subscriptions.go — управление подписками клиента на категории уведомлений.
package portal

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"
)

// availableCategories — полный список категорий, доступных для подписки.
var availableCategories = []subscriptionCategory{
	{Key: "regulations", Label: "Нормативные документы", Desc: "Изменения в НПА, льготах и регламентах Сколково"},
	{Key: "events", Label: "Мероприятия", Desc: "Новые и изменённые мероприятия Сколково"},
	{Key: "contests", Label: "Конкурсы и гранты", Desc: "Новые конкурсы, изменения статусов, итоги"},
	{Key: "reporting", Label: "Отчётность", Desc: "Изменения в требованиях к отчётности резидентов"},
}

type subscriptionCategory struct {
	Key      string
	Label    string
	Desc     string
	Checked  bool
}

type subscriptionsData struct {
	Client               interface{} // *model.Client
	Categories           []subscriptionCategory
	Flash                string
	FlashKind            string
	ActiveTabSubscriptions bool
}

func (ps *PortalServer) handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	client, err := ps.getClient(r.Context(), sess.ClientID)
	if err != nil {
		http.Error(w, "Клиент не найден", http.StatusNotFound)
		return
	}

	subscribed := map[string]bool{}
	if ps.stores.SubscriptionStore != nil {
		cats, _ := ps.stores.SubscriptionStore.GetSubscriptions(r.Context(), client.ID)
		for _, c := range cats {
			subscribed[c] = true
		}
	}

	cats := make([]subscriptionCategory, len(availableCategories))
	for i, c := range availableCategories {
		cats[i] = c
		cats[i].Checked = subscribed[c.Key]
	}

	data := subscriptionsData{
		Client:                client,
		Categories:            cats,
		Flash:                 r.URL.Query().Get("msg"),
		ActiveTabSubscriptions: true,
	}
	if data.Flash != "" {
		data.FlashKind = "ok"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := portalTmpl.ExecuteTemplate(w, "subscriptions", data); err != nil {
		log.Println("[portal] шаблон subscriptions:", err)
	}
}

func (ps *PortalServer) handleSubscriptionsSubmit(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r)
	client, err := ps.getClient(r.Context(), sess.ClientID)
	if err != nil {
		http.Error(w, "Клиент не найден", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/subscriptions", http.StatusSeeOther)
		return
	}

	// Собираем выбранные чекбоксы.
	var selected []string
	validKeys := make(map[string]bool)
	for _, c := range availableCategories {
		validKeys[c.Key] = true
	}
	for _, v := range r.Form["categories"] {
		if validKeys[strings.TrimSpace(v)] {
			selected = append(selected, strings.TrimSpace(v))
		}
	}

	if ps.stores.SubscriptionStore != nil {
		if err := ps.stores.SubscriptionStore.SetSubscriptions(r.Context(), client.ID, selected); err != nil {
			log.Printf("[portal] SetSubscriptions %s: %v", client.ID, err)
			http.Redirect(w, r, "/subscriptions?msg="+html.EscapeString("Ошибка сохранения"), http.StatusSeeOther)
			return
		}
	}

	n := len(selected)
	msg := fmt.Sprintf("Подписки обновлены: %d категор", n)
	switch {
	case n%10 == 1 && n%100 != 11:
		msg += "ия"
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 10 || n%100 >= 20):
		msg += "ии"
	default:
		msg += "ий"
	}
	http.Redirect(w, r, "/subscriptions?msg="+html.EscapeString(msg), http.StatusSeeOther)
}
