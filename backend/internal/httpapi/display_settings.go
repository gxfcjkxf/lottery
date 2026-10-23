package httpapi

import "net/http"

// Only display policy is public. Administrative metadata and audit IDs stay private.
func registerPublicDisplaySettings(mux *routeMux, d Dependencies) {
	mux.HandleFunc("GET /api/v1/platform/presentation", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var out struct {
			DisplayName string `json:"display_name"`
			LogoText    string `json:"logo_text"`
			LogoURL     string `json:"logo_url"`
		}
		if d.Admins.DB == nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法加载平台展示设置")
			return
		}
		err := d.Admins.DB.QueryRow(r.Context(), `SELECT display_name,logo_text,logo_url FROM platform_settings WHERE id=true`).Scan(&out.DisplayName, &out.LogoText, &out.LogoURL)
		if err != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法加载平台展示设置")
			return
		}
		respond(w, r, 200, out)
	})
	mux.HandleFunc("GET /api/v1/display-settings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var out struct {
			Timezone   string `json:"timezone"`
			DateFormat string `json:"date_format"`
			TimeFormat string `json:"time_format"`
		}
		if d.Admins.DB == nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法加载显示设置")
			return
		}
		err := d.Admins.DB.QueryRow(r.Context(), `SELECT timezone,date_format,time_format FROM platform_settings WHERE id=true`).Scan(&out.Timezone, &out.DateFormat, &out.TimeFormat)
		if err != nil {
			failure(w, r, 503, "SERVICE_UNAVAILABLE", "暂时无法加载显示设置")
			return
		}
		respond(w, r, 200, out)
	})
}
