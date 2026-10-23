package rules

import "embed"

// Built-in templates are schema-v2 data. No sale or settlement dispatches on
// the template name, and existing database rule/sales stores are never imported.
//
//go:embed config/mark_six_v2.json config/canada_*.json
var configuredTemplates embed.FS

var ConfiguredTemplateNames = []string{"mark_six", "canada_28", "canada_996", "canada_vip", "canada_42", "canada_web", "canada_50", "canada_baccarat", "canada_niuniu"}

func ConfiguredTemplateCatalog(name string) (ConfiguredCatalog, error) {
	filename := ""
	for _, allowed := range ConfiguredTemplateNames {
		if name == allowed {
			filename = "config/" + name + ".json"
			break
		}
	}
	if filename == "" {
		return ConfiguredCatalog{}, ErrInvalid
	}
	if name == "mark_six" {
		filename = "config/mark_six_v2.json"
	}
	raw, err := configuredTemplates.ReadFile(filename)
	if err != nil {
		return ConfiguredCatalog{}, err
	}
	return DecodeConfiguredCatalog(raw)
}
