package httpapi

import (
	"net/http"
	"sort"
	"strings"
)

// The inventory observes the same HandleFunc registrations used by production.
// It does not invoke handlers or require a database, credentials or network.
type Route struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}
type routeMux struct {
	*http.ServeMux
	routes []Route
}
type routeRegistrar interface {
	HandleFunc(string, func(http.ResponseWriter, *http.Request))
}

func newRouteMux() *routeMux { return &routeMux{ServeMux: http.NewServeMux()} }
func (m *routeMux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	m.ServeMux.HandleFunc(pattern, handler)
	method, path, ok := strings.Cut(pattern, " ")
	if ok {
		m.routes = append(m.routes, Route{method, path})
	}
}
func RegisteredRoutes() []Route {
	m := buildRouter(Dependencies{})
	out := append([]Route(nil), m.routes...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
}
