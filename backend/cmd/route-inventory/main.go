// route-inventory reads actual registrations without starting any services.
package main

import (
	"encoding/json"
	"github.com/gxfcjkxf/lottery/backend/internal/httpapi"
	"os"
)

func main() {
	if e := json.NewEncoder(os.Stdout).Encode(httpapi.RegisteredRoutes()); e != nil {
		os.Exit(1)
	}
}
