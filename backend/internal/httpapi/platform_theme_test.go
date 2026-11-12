package httpapi

import (
 "testing"
 "github.com/gxfcjkxf/lottery/backend/internal/platformsettings"
)

func TestPlatformThemePermissionVersionAndReplay(t *testing.T) {
 f := managedFixture(t)
 op := securityOperator(t,f)
 path := "/api/v1/platform/platform-settings"
 mustStatus(t,op.call("GET",path,"",op.token,"",nil),403)
 grantPlatformPermission(t,op,"platform_settings.view.platform","platform_settings.write.platform")
 var initial platformsettings.Settings
 managedData(t,op.call("GET",path,"",op.token,"",nil),&initial)
 in := platformsettings.Input{Version:initial.Version,DisplayName:initial.DisplayName,LogoText:initial.LogoText,LogoURL:initial.LogoURL,Timezone:initial.Timezone,DateFormat:initial.DateFormat,TimeFormat:initial.TimeFormat,Theme:"ocean"}
 mustStatus(t,f.call("PATCH","/api/v1/admin/platform-settings","theme-brand-denied",f.token,managedBrand,in),404)
 first := op.call("PATCH",path,"theme-save",op.token,"",in)
 mustStatus(t,first,200)
 replay := op.call("PATCH",path,"theme-save",op.token,"",in)
 mustStatus(t,replay,200)
 var saved,again platformsettings.Settings
 managedData(t,first,&saved);managedData(t,replay,&again)
 if saved.Theme!="ocean" || saved.Version!=initial.Version+1 || again.Version!=saved.Version { t.Fatal(saved,again) }
 mustStatus(t,op.call("PATCH",path,"theme-stale",op.token,"",in),409)
 in.Version=saved.Version;in.Theme="unknown"
 mustStatus(t,op.call("PATCH",path,"theme-invalid",op.token,"",in),400)
 var display map[string]string
 managedData(t,f.call("GET","/api/v1/display-settings","","","",nil),&display)
 if display["theme"]!="ocean" { t.Fatal(display) }
}
