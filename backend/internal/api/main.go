package api

import (
	"encoding/json"
	"net/http"
)

func healthHandler(w http.ResponseWriter ,r *http.Request){
	w.Header().Set("Content-type", "application/json")

	json.NewEncoder(w).Encode(map[string]string{
		"status":"ok",
	})
}

func AuthHandler(w http.ResponseWriter, r *http.Request){

}
func NewRouter() *http.ServeMux {
    mux := http.NewServeMux()
	mux.HandleFunc("/health" , healthHandler)
	mux.HandleFunc("GET /api/v1/Auth", AuthHandler)

    return mux
}
func main() {
	router := NewRouter()
	http.ListenAndServe(":8080", router)
}