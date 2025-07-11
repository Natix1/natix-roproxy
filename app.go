package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

var (
	PROXY_KEY   string
	HTTP_CLIENT *http.Client
	SERIAL      int
)

const (
	PORT           = 8000
	REAL_IP_HEADER = ""
)

func init() {
	godotenv.Load()

	val, ok := os.LookupEnv("PROXY_KEY")
	if !ok {
		log.Fatal("PROXY_KEY not specified. Set it in .env.")
	}

	PROXY_KEY = val
	HTTP_CLIENT = &http.Client{}
}

func get_real_ip(r *http.Request) string {
	var finalIP string

	if REAL_IP_HEADER == "" {
		finalIP = r.RemoteAddr
	} else {
		REAL_IP := r.Header.Get(REAL_IP_HEADER)

		if REAL_IP == "" {
			finalIP = r.RemoteAddr
		}
	}

	if strings.HasPrefix(finalIP, "127.0.0.1") {
		finalIP = "localhost" + strings.TrimPrefix(finalIP, "127.0.0.1")
	}

	return finalIP
}

func make_request(url string, r *http.Request, forwarded_for string) ([]byte, int, error) {
	req, err := http.NewRequest(r.Method, url, r.Body)
	if err != nil {
		return []byte(""), -1, err
	}

	req.Header = r.Header

	// Overwrite some headers
	req.Header.Set("Natix-Roproxy-Forwarded-For", forwarded_for)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Natix-Roproxy")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("CF-Connecting-IP", "")

	resp, err := HTTP_CLIENT.Do(req)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	return respBody, resp.StatusCode, nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("x-proxy-key") != PROXY_KEY {
		http.Error(w, "Invalid proxy key supplied!", http.StatusUnauthorized)
		return
	}

	url_parts := strings.Split(r.URL.Path, "/")

	// above parses something.com/subdomain/arg1/arg2 to ['subdomain', 'arg1', 'arg2']. so at least 3
	if len(url_parts) < 3 {
		http.Error(w, "Invalid URL path. Format is: (host)/roblox_subdomain/actual/path", http.StatusBadRequest)
		return
	}

	// increment serial
	SERIAL++

	roblox_subdomain := url_parts[1]
	var urlbuilder strings.Builder
	urlbuilder.WriteString(fmt.Sprintf("https://%s.roblox.com", roblox_subdomain))

	for _, part := range url_parts[2:] {
		urlbuilder.WriteString("/")
		urlbuilder.WriteString(part)
	}

	roblox_url := urlbuilder.String()

	if _, err := url.ParseRequestURI(roblox_url); err != nil {
		http.Error(w, "Error while building URL", http.StatusInternalServerError)
		return
	}

	log.Printf("(SRL-%d) %s -> %s\n", SERIAL, get_real_ip(r), roblox_url)
	resp, status_code, err := make_request(roblox_url, r, get_real_ip(r))

	if err != nil {
		log.Printf("(SRL-%d) Error while making request to %s: %s", SERIAL, roblox_url, err.Error())
		http.Error(w, "Proxy encountered error while making request to roblox: "+err.Error(), status_code)
	}

	w.Write(resp)
}

func main() {
	http.HandleFunc("/", handler)
	http.ListenAndServe("0.0.0.0:"+strconv.Itoa(PORT), nil)
}
