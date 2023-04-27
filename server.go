package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
)

const (
	apiKey    = ""
	apiSecret = ""
)

type BitstampBalance struct {
	Balance float64 `json:"balance"`
}

func main() {
	http.HandleFunc("/balance", balanceHandler)
	http.ListenAndServe(":8080", nil)
}

func balanceHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Generate a nonce for the Bitstamp API request
	nonce := strconv.FormatInt(time.Now().UnixNano(), 10)

	// Generate the message to sign
	message := nonce + apiKey

	// Sign the message using the API secret
	signature := hmac.New(sha256.New, []byte(apiSecret))
	signature.Write([]byte(message))
	signatureHex := hex.EncodeToString(signature.Sum(nil))

	// Create the HTTP request to the Bitstamp API
	url := "https://www.bitstamp.net/api/v2/balance/"
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		http.Error(w, "Error creating Bitstamp API request", http.StatusInternalServerError)
		return
	}

	// Set the HTTP request headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("X-Auth", "BITSTAMP " + apiKey)
	req.Header.Set("X-Auth-Signature", signatureHex)
	req.Header.Set("X-Auth-Nonce", nonce)

	// Send the HTTP request to the Bitstamp API and retrieve the response
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Error sending Bitstamp API request", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Parse the JSON response from the Bitstamp API and extract the account balance
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Error reading Bitstamp API response", http.StatusInternalServerError)
		return
	}

	var data map[string]interface{}
	err = json.Unmarshal(body, &data)
	if err != nil {
		http.Error(w, "Error parsing Bitstamp API response", http.StatusInternalServerError)
		return
	}

	balance, ok := data["usd_balance"].(string)
	if !ok {
		http.Error(w, "Error extracting account balance", http.StatusInternalServerError)
		return
	}

	balanceFloat, err := strconv.ParseFloat(balance, 64)
	if err != nil {
		http.Error(w, "Error converting account balance to float", http.StatusInternalServerError)
		return
	}

	// Set the CORS headers on the HTTP response
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Return the account balance as a JSON object
	json.NewEncoder(w).Encode(BitstampBalance{Balance: balanceFloat})
}
