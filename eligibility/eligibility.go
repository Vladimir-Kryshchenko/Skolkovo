// Package eligibility проверяет соответствие компании требованиям резидентства Сколково
// по ИНН: существование юрлица (ЕГРЮЛ), статус МСП, действующий статус.
package eligibility

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Config — конфигурация проверки eligibility.
type Config struct {
	EGRULSearchURL string // URL поиска по ИНН в ЕГРЮЛ (dadata.ru или egrul.nalog.ru)
	MSPCheckURL    string // URL реестра МСП (rmsp.nalog.ru)
	DadataAPIKey   string // API-ключ DaData (если используется)
	HTTPTimeout    time.Duration
}

// Checker проверяет eligibility компании по ИНН.
type Checker struct {
	cfg  Config
	http *http.Client
}

// CompanyInfo — информация о компании из ЕГРЮЛ.
type CompanyInfo struct {
	INN        string    `json:"inn"`
	OGRN       string    `json:"ogrn"`
	FullName   string    `json:"full_name"`
	ShortName  string    `json:"short_name"`
	Status     string    `json:"status"` // "active" | "liquidating" | "liquidated" | "bankrupt"
	OrgType    string    `json:"org_type"` // "ООО" | "АО" | "ПАО" | "ИП" и т.д.
	RegDate    time.Time `json:"reg_date"`
	Address    string    `json:"address"`
	OKVED      string    `json:"okved_main"` // основной ОКВЭД
	OKVEDName  string    `json:"okved_name"`
	IsMSP      bool      `json:"is_msp"`      // является ли МСП
	MSPCategory string   `json:"msp_category"` // "micro" | "small" | "medium"
}

// EligibilityReport — отчёт о соответствии компании требованиям Сколково.
type EligibilityReport struct {
	INN           string        `json:"inn"`
	Company       *CompanyInfo  `json:"company,omitempty"`
	Eligible      bool          `json:"eligible"`
	Score         int           `json:"score"` // 0-100
	Issues        []string      `json:"issues"`
	Warnings      []string      `json:"warnings"`
	Recommendations []string    `json:"recommendations"`
	CheckedAt     time.Time     `json:"checked_at"`
}

// NewChecker создаёт новый Checker.
func NewChecker(cfg Config) *Checker {
	timeout := cfg.HTTPTimeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	return &Checker{
		cfg:  cfg,
		http: &http.Client{Timeout: timeout},
	}
}

// CheckByINN проверяет компанию по ИНН и возвращает отчёт о соответствии.
func (c *Checker) CheckByINN(ctx context.Context, inn string) (*EligibilityReport, error) {
	inn = strings.TrimSpace(inn)
	if err := validateINN(inn); err != nil {
		return nil, fmt.Errorf("некорректный ИНН: %w", err)
	}

	report := &EligibilityReport{
		INN:       inn,
		CheckedAt: time.Now(),
	}

	// 1. Получаем данные о компании из ЕГРЮЛ.
	info, err := c.lookupEGRUL(ctx, inn)
	if err != nil {
		// Если ЕГРЮЛ недоступен, работаем с ограниченной проверкой.
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("ЕГРЮЛ временно недоступен (%v), проверка ограничена", err))
	} else {
		report.Company = info
		// Проверяем статус МСП.
		if isMSP, cat, err := c.checkMSP(ctx, inn); err == nil {
			report.Company.IsMSP = isMSP
			report.Company.MSPCategory = cat
		}
	}

	// 2. Анализируем соответствие требованиям.
	c.analyzeEligibility(report)

	return report, nil
}

// lookupEGRUL запрашивает данные о компании в ЕГРЮЛ.
func (c *Checker) lookupEGRUL(ctx context.Context, inn string) (*CompanyInfo, error) {
	// Пробуем DaData API если есть ключ — самый надёжный источник.
	if c.cfg.DadataAPIKey != "" {
		return c.lookupViaDadata(ctx, inn)
	}
	// Иначе — публичный API ФНС.
	return c.lookupViaFNS(ctx, inn)
}

// lookupViaDadata получает данные через DaData API (https://dadata.ru).
func (c *Checker) lookupViaDadata(ctx context.Context, inn string) (*CompanyInfo, error) {
	apiURL := "https://suggestions.dadata.ru/suggestions/api/4_1/rs/findById/party"
	payload := fmt.Sprintf(`{"query":%q,"count":1}`, inn)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL,
		strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Token "+c.cfg.DadataAPIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DaData API: статус %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseDadataResponse(inn, body)
}

// dadataSuggestions — ответ DaData API.
type dadataSuggestions struct {
	Suggestions []struct {
		Value string `json:"value"`
		Data  struct {
			INN       string `json:"inn"`
			OGRN      string `json:"ogrn"`
			Name      struct {
				Full  string `json:"full_with_opf"`
				Short string `json:"short_with_opf"`
			} `json:"name"`
			State struct {
				Status string `json:"status"` // ACTIVE, LIQUIDATING, LIQUIDATED, BANKRUPT
			} `json:"state"`
			OpfType string `json:"opf"`
			RegistrationDate int64 `json:"registration_date"`
			Address struct {
				Value string `json:"value"`
			} `json:"address"`
			OKVEDCode string `json:"okved"`
			OKVEDText string `json:"okved_type"`
		} `json:"data"`
	} `json:"suggestions"`
}

func parseDadataResponse(inn string, body []byte) (*CompanyInfo, error) {
	var resp dadataSuggestions
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Suggestions) == 0 {
		return nil, fmt.Errorf("компания с ИНН %s не найдена", inn)
	}
	s := resp.Suggestions[0].Data

	info := &CompanyInfo{
		INN:      s.INN,
		OGRN:     s.OGRN,
		FullName: s.Name.Full,
		ShortName: s.Name.Short,
		OrgType:  s.OpfType,
		Address:  s.Address.Value,
		OKVED:    s.OKVEDCode,
		OKVEDName: s.OKVEDText,
	}

	if s.RegistrationDate > 0 {
		info.RegDate = time.Unix(s.RegistrationDate/1000, 0)
	}

	switch s.State.Status {
	case "ACTIVE":
		info.Status = "active"
	case "LIQUIDATING":
		info.Status = "liquidating"
	case "LIQUIDATED":
		info.Status = "liquidated"
	case "BANKRUPT":
		info.Status = "bankrupt"
	default:
		info.Status = strings.ToLower(s.State.Status)
	}

	return info, nil
}

// lookupViaFNS запрашивает ЕГРЮЛ через публичный поиск ФНС.
func (c *Checker) lookupViaFNS(ctx context.Context, inn string) (*CompanyInfo, error) {
	baseURL := c.cfg.EGRULSearchURL
	if baseURL == "" {
		baseURL = "https://egrul.nalog.ru/"
	}

	// Шаг 1: получаем токен для запроса.
	tokenURL := baseURL + "api/v1/search/"
	formData := url.Values{}
	formData.Set("query", inn)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL,
		strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", eligibilityUserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseFNSResponse(inn, body)
}

// fnsSearchResult — результат поиска ФНС.
type fnsSearchResult struct {
	Token  string `json:"t"`
	Total  int    `json:"total"`
	Rows   []struct {
		G      string `json:"g"` // краткое наименование
		N      string `json:"n"` // полное наименование
		C      string `json:"c"` // ИНН
		O      string `json:"o"` // ОГРН
		State  string `json:"state"`
		R      string `json:"r"` // дата регистрации
		A      string `json:"a"` // адрес
		T      string `json:"t"` // тип
	} `json:"rows"`
}

func parseFNSResponse(inn string, body []byte) (*CompanyInfo, error) {
	var result fnsSearchResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if len(result.Rows) == 0 {
		return nil, fmt.Errorf("компания с ИНН %s не найдена в ЕГРЮЛ", inn)
	}
	row := result.Rows[0]

	info := &CompanyInfo{
		INN:       inn,
		OGRN:      row.O,
		FullName:  row.N,
		ShortName: row.G,
		OrgType:   row.T,
		Address:   row.A,
		Status:    normalizeStatus(row.State),
	}

	if row.R != "" {
		if t, err := time.Parse("02.01.2006", row.R); err == nil {
			info.RegDate = t
		}
	}

	return info, nil
}

func normalizeStatus(s string) string {
	switch strings.ToUpper(s) {
	case "ДЕЙСТВУЮЩЕЕ", "ACTIVE":
		return "active"
	case "В ПРОЦЕССЕ ЛИКВИДАЦИИ", "LIQUIDATING":
		return "liquidating"
	case "ЛИКВИДИРОВАНО", "LIQUIDATED":
		return "liquidated"
	case "БАНКРОТ", "BANKRUPT":
		return "bankrupt"
	default:
		return strings.ToLower(s)
	}
}

// mspCheckResult — результат проверки МСП.
type mspCheckResult struct {
	Category string `json:"category"` // micro, small, medium
	IsMSP    bool   `json:"is_msp"`
}

// checkMSP проверяет наличие компании в реестре МСП ФНС.
func (c *Checker) checkMSP(ctx context.Context, inn string) (bool, string, error) {
	checkURL := c.cfg.MSPCheckURL
	if checkURL == "" {
		checkURL = "https://rmsp.nalog.ru/api/v1/search/"
	}

	formData := url.Values{}
	formData.Set("query", inn)
	formData.Set("typeSearch", "inn")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, checkURL,
		strings.NewReader(formData.Encode()))
	if err != nil {
		return false, "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", eligibilityUserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("МСП API: статус %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, "", err
	}

	var result struct {
		Total int `json:"total"`
		Rows  []struct {
			Category string `json:"category"` // MICRO, SMALL, MEDIUM
		} `json:"rows"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return false, "", err
	}

	if result.Total == 0 || len(result.Rows) == 0 {
		return false, "", nil
	}

	cat := strings.ToLower(result.Rows[0].Category)
	return true, cat, nil
}

// analyzeEligibility анализирует собранные данные и формирует заключение.
func (c *Checker) analyzeEligibility(report *EligibilityReport) {
	score := 100
	info := report.Company

	if info == nil {
		// Нет данных — не можем ничего подтвердить.
		report.Issues = append(report.Issues, "Не удалось получить данные из ЕГРЮЛ")
		report.Score = 0
		report.Eligible = false
		return
	}

	// Проверка 1: компания должна быть активна.
	switch info.Status {
	case "liquidated":
		report.Issues = append(report.Issues, "Компания ликвидирована — не может получить статус резидента")
		score -= 100
	case "liquidating":
		report.Issues = append(report.Issues, "Компания находится в процессе ликвидации")
		score -= 80
	case "bankrupt":
		report.Issues = append(report.Issues, "Компания признана банкротом — не может получить статус резидента")
		score -= 100
	}

	// Проверка 2: тип организации.
	// ИП не могут быть резидентами Сколково, только юрлица.
	if strings.Contains(strings.ToUpper(info.OrgType), "ИП") ||
		strings.Contains(strings.ToUpper(info.FullName), "ИНДИВИДУАЛЬНЫЙ ПРЕДПРИНИМАТЕЛЬ") {
		report.Issues = append(report.Issues, "Индивидуальные предприниматели не могут получить статус резидента Сколково")
		score -= 100
	}

	// Проверка 3: возраст компании.
	if !info.RegDate.IsZero() {
		ageMonths := int(time.Since(info.RegDate).Hours() / 24 / 30)
		if ageMonths < 1 {
			report.Warnings = append(report.Warnings,
				"Компания зарегистрирована менее 1 месяца назад — может потребоваться дополнительное время для оформления документов")
		}
	}

	// Проверка 4: ОКВЭД должен соответствовать инновационной деятельности.
	if info.OKVED != "" {
		if !isInnovativeOKVED(info.OKVED) {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("Основной ОКВЭД %s (%s) не является инновационным. "+
					"Для Сколково требуется деятельность в сфере науки, технологий, IT, биотехнологий, "+
					"ядерных технологий, космических технологий или телекоммуникаций",
					info.OKVED, info.OKVEDName))
			score -= 20
		}
	}

	// Проверка 5: МСП статус.
	// Формально МСП не требуется для Сколково, но часть грантов ограничена МСП.
	if !info.IsMSP {
		report.Warnings = append(report.Warnings,
			"Компания не входит в реестр МСП. Некоторые гранты Сколково доступны только для МСП")
	}

	// Рекомендации.
	if info.Status == "active" {
		report.Recommendations = append(report.Recommendations,
			"Компания активна — можно подавать заявку на резидентство",
			"Подготовьте описание инновационного проекта (НИОКР) согласно требованиям Сколково",
			"Убедитесь, что деятельность соответствует одному из кластеров: IT, биомед, энергетика, космос, ядерные технологии",
		)
	}

	if score < 0 {
		score = 0
	}
	report.Score = score
	report.Eligible = score >= 50 && len(report.Issues) == 0
}

// isInnovativeOKVED проверяет, относится ли ОКВЭД к инновационной деятельности,
// подходящей для резидентства Сколково.
func isInnovativeOKVED(okved string) bool {
	// Подходящие разделы ОКВЭД:
	// 62, 63 — IT, разработка ПО, обработка данных
	// 72 — Научные исследования
	// 71 — Техническое тестирование и исследования
	// 86 — Деятельность в области здравоохранения (биомед)
	// 64-66 — Финансовые технологии (fintech)
	// 26 — Производство электроники
	// 27 — Производство электрооборудования
	// 28 — Производство машин и оборудования
	// 21 — Фармацевтика
	// 30 — Производство транспортных средств
	innovativePrefixes := []string{
		"62", "63", "72", "71", "86.21", "86.22", "86.23",
		"64", "65", "66", "26", "27", "28", "21", "30",
		"61", "58.2", "59", "60", "74.9",
	}
	for _, prefix := range innovativePrefixes {
		if strings.HasPrefix(okved, prefix) {
			return true
		}
	}
	return false
}

// validateINN проверяет формат ИНН.
func validateINN(inn string) error {
	if len(inn) != 10 && len(inn) != 12 {
		return fmt.Errorf("ИНН должен содержать 10 (юрлицо) или 12 (ИП) цифр, получено %d", len(inn))
	}
	for _, r := range inn {
		if r < '0' || r > '9' {
			return fmt.Errorf("ИНН должен содержать только цифры")
		}
	}
	return nil
}

// SkolkovoClusters возвращает список кластеров Сколково для информирования.
func SkolkovoClusters() []string {
	return []string{
		"IT (информационные технологии)",
		"Биомедицинские технологии",
		"Энергоэффективные технологии",
		"Космические технологии и телекоммуникации",
		"Ядерные технологии",
	}
}

const eligibilityUserAgent = "Mozilla/5.0 (compatible; SkolkovoBase/1.0; +https://sk.ru)"
