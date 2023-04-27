package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const (
	baseURL = "https://www.bitstamp.net/api/v2/"
)

// var nonceCounter int64 = 1
var nonceCounter int64 = time.Now().Unix()

// Read API credentials from environment variables
var (
	apiKey     = os.Getenv("BITSTAMP_API_KEY")
	apiSecret  = os.Getenv("BITSTAMP_API_SECRET")
	customerID = os.Getenv("BITSTAMP_CUSTOMER_ID")
)

func main() {
	http.HandleFunc("/balance", handleBalance)
	fmt.Println("Server listening on port 8080")
	http.ListenAndServe(":8080", nil)
}

func handleBalance(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	accountBalance, err := getAccountBalance()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Error fetching account balance"}`))
		return
	}

	jsonBalance, err := json.Marshal(accountBalance)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "Error converting account balance to JSON"}`))
		return
	}

	w.Write(jsonBalance)
}

func getAccountBalance() (map[string]interface{}, error) {
	url := baseURL + "balance/"
	nonce := strconv.FormatInt(atomic.AddInt64(&nonceCounter, 1), 10)

	message := nonce + customerID + apiKey
	signature := getSignature(message)

	payload := "key=" + apiKey + "&signature=" + signature + "&nonce=" + nonce
	fmt.Println("Payload:", payload) // Debugging: Print payload

	req, err := http.NewRequest("POST", url, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/4.0 (compatible; Bitstamp Golang client)")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	fmt.Println("Response:", string(body)) // Debugging: Print response

	var balance map[string]interface{}
	err = json.Unmarshal(body, &balance)
	if err != nil {
		return nil, err
	}

	return balance, nil
}

func getSignature(message string) string {
	mac := hmac.New(sha256.New, []byte(apiSecret))
	mac.Write([]byte(message))
	return strings.ToUpper(hex.EncodeToString(mac.Sum(nil)))
}
