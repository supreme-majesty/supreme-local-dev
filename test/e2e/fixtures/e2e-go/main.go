// Proves the go driver (`go run .`), privilege drop and routing.
package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "E2E-GO-OK uid=%d\n", os.Getuid())
	})
	if err := http.ListenAndServe("0.0.0.0:"+os.Getenv("PORT"), nil); err != nil {
		panic(err)
	}
}
