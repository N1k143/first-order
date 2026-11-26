package discovery

import (
	"net/mail"
	"regexp"
	"strings"
)

type CatalogReadiness struct {
	Ready        bool     `json:"ready"`
	Reasons      []string `json:"reasons"`
	CategoryName string   `json:"category_name"`
	Phones       []string `json:"phones"`
	Email        *string  `json:"email"`
}

var internationalPhone = regexp.MustCompile(`\+?[78](?:[ ()\.\-]*[0-9]){10}`)

func CatalogPhones(raw string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ';' || r == ',' }) {
		for _, span := range internationalPhone.FindAllStringIndex(part, -1) {
			if (span[0] > 0 && part[span[0]-1] >= '0' && part[span[0]-1] <= '9') || (span[1] < len(part) && part[span[1]] >= '0' && part[span[1]] <= '9') {
				continue
			}
			match := part[span[0]:span[1]]
			number := digits(match)
			if len(number) == 11 {
				number = "+7" + number[1:]
				if !seen[number] {
					seen[number] = true
					result = append(result, number)
				}
			}
		}
	}
	return result
}
func CatalogCategory(raw string) string {
	groups := []struct {
		Name string
		Keys string
	}{
		{"Банки и финансовые услуги", "amenity:bank office:financial office:financial_advisor office:insurance"},
		{"Кафе, бары и рестораны", "amenity:bar amenity:cafe amenity:fast_food amenity:pub amenity:restaurant craft:caterer shop:coffee"},
		{"Автоуслуги", "amenity:car_rental amenity:car_wash amenity:fuel shop:car shop:car_parts shop:car_repair"},
		{"Культура и развлечения", "amenity:cinema amenity:theatre"},
		{"Медицина и аптеки", "amenity:clinic amenity:dentist amenity:pharmacy shop:chemist"},
		{"Образование", "amenity:driving_school amenity:language_school office:educational_institution"},
		{"Ветеринарные услуги", "amenity:veterinary"},
		{"Строительство и ремонт", "craft:ceiling craft:electrician craft:hvac craft:window_construction office:construction_company shop:doityourself shop:doors shop:electrical shop:flooring"},
		{"Компьютеры, электроника и сервис", "craft:computers craft:electronics_repair office:it shop:appliance shop:computer shop:electronics"},
		{"Фотоуслуги", "craft:photographer"},
		{"Ремонт одежды и обуви", "craft:shoemaker craft:tailor"},
		{"Бухгалтерия и консалтинг", "office:accountant office:consulting"},
		{"Реклама и полиграфия", "office:advertising_agency office:publisher shop:copyshop"},
		{"Офисы и деловые услуги", "office:company office:coworking office:yes"},
		{"Недвижимость и обслуживание зданий", "office:estate_agent office:property_management"},
		{"Туризм и переводы", "office:guide office:travel_agent office:translator"},
		{"Юридические услуги", "office:lawyer office:notary"},
		{"Логистика", "office:logistics"},
		{"СМИ", "office:newspaper"},
		{"Охрана и безопасность", "office:security"},
		{"Уборка и бытовые услуги", "shop:dry_cleaning"},
		{"Связь", "office:telecommunication"},
		{"Товары для дома и мебель", "shop:baby_goods shop:bed shop:carpet shop:fabric shop:furniture"},
		{"Продукты и напитки", "shop:alcohol shop:bakery shop:beverages shop:butcher shop:chocolate shop:confectionery shop:convenience shop:farm shop:dairy"},
		{"Красота и косметика", "shop:beauty shop:cosmetics"},
		{"Спорт и отдых", "shop:bicycle shop:fishing"},
		{"Книги", "shop:books"},
		{"Одежда и аксессуары", "shop:clothes shop:fashion_accessories"},
		{"Магазины", "shop:department_store"},
		{"Цветы, сад и сельское хозяйство", "shop:agrarian shop:florist shop:garden_centre"},
		{"Ритуальные услуги", "shop:funeral_directors"},
		{"Специализированные магазины", "shop:bookmaker shop:e-cigarette shop:erotic"},
	}
	for _, group := range groups {
		for _, key := range strings.Fields(group.Keys) {
			if raw == key {
				return group.Name
			}
		}
	}
	return ""
}
func AssessCatalog(c Candidate) CatalogReadiness {
	result := CatalogReadiness{Reasons: []string{}, CategoryName: CatalogCategory(c.Category), Phones: CatalogPhones(c.Phone)}
	for _, key := range []string{"office:diplomatic", "office:government", "office:political_party", "office:ngo", "office:water_utility"} {
		if c.Category == key {
			result.Reasons = append(result.Reasons, "not_commercial_company")
		}
	}
	if strings.TrimSpace(c.Name) == "" || c.City != "Астана" || !strings.HasPrefix(c.SourceURL, "https://www.openstreetmap.org/") {
		result.Reasons = append(result.Reasons, "invalid_identity_or_source")
	}
	if result.CategoryName == "" {
		result.Reasons = append(result.Reasons, "category_needs_review")
	}
	if address, err := mail.ParseAddress(strings.TrimSpace(c.Email)); err == nil && address.Address == strings.TrimSpace(c.Email) && strings.Contains(address.Address, "@") {
		email := address.Address
		result.Email = &email
	}
	verifiedWebsite := false
	if c.Check.Status == "found" {
		link := c.Check.FinalURL
		if link == "" {
			link = c.WebsiteURL
		}
		_, err := validTarget(link)
		verifiedWebsite = err == nil
	}
	if len(result.Phones) == 0 && result.Email == nil && !verifiedWebsite {
		result.Reasons = append(result.Reasons, "no_usable_contact")
	}
	result.Ready = len(result.Reasons) == 0
	return result
}
