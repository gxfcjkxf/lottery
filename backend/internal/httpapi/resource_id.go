package httpapi

import "net/http"

func ruleID(w http.ResponseWriter, r *http.Request, key string) (string, bool) {
	id := r.PathValue(key)
	if !uuidPattern.MatchString(id) {
		failure(w, r, 400, "REQUEST_INVALID", "记录编号不正确")
		return id, false
	}
	return id, true
}
