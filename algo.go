package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {

	for {

		var err error

		const bitstamphost string = `www.bitstamp.net`

		const valrhost string = `api.valr.com`

		var accounts [][]string

		if accounts, err = ReadCsv(os.Args[1]); err != nil {

			log.Panic(err)

			return
		}

		var index int = 0

		for index = range accounts {

			var account []string = accounts[index]

			var bitstampkey string = account[0]
			var bitstampsecret string = account[1]
			var bitstampcustomer string = account[2]

			var valrkey string = account[3]
			var valrsecret string = account[4]

			var exchangerate float64
			var profitmargin float64
			var executetrade bool

			if exchangerate, err = strconv.ParseFloat(account[5], 64); err != nil {

				log.Panic(err)

				return
			}

			if profitmargin, err = strconv.ParseFloat(account[6], 64); err != nil {

				log.Panic(err)

				return
			}

			if executetrade, err = strconv.ParseBool(account[7]); err != nil {

				log.Panic(err)

				return
			}

			var bitstampdollarbalance float64 = GetBitstampDollarBalance(bitstampkey, bitstampsecret, bitstampcustomer, bitstamphost)
			var valrbitcoinbalance float64 = GetValrBitcoinBalance(valrkey, valrsecret, valrhost)

			log.Printf(`bitstampdollarbalance: %+[1]v`, bitstampdollarbalance)
			log.Printf(`valrbitcoinbalance: %+[1]v`, valrbitcoinbalance)

			bitstampdollarbalance *= 0.02
			valrbitcoinbalance *= 0.02

			log.Printf(`bitstampdollarbalance: %+[1]v`, bitstampdollarbalance)
			log.Printf(`valrbitcoinbalance: %+[1]v`, valrbitcoinbalance)

			var bitstampbuyable Depth = GetBitstampBuyableLiquidity(bitstampkey, bitstampsecret, bitstampcustomer, bitstamphost)

			var bitstamptrade Trade = CalculateTrade(bitstampbuyable, bitstampdollarbalance)

			if bitstamptrade.NotionalAmount == bitstampdollarbalance {

				var valrrandnotional float64 = bitstamptrade.NotionalAmount * exchangerate

				var valrsellable Depth = GetValrSellableLiquidity(valrkey, valrsecret, valrhost)

				var valrtrade Trade = CalculateTrade(valrsellable, valrrandnotional)

				if valrtrade.NotionalAmount == valrrandnotional && valrtrade.BaseAmount <= valrbitcoinbalance {

					var bitcoinprofitpercent float64 = CalculateProfit(bitstamptrade.BaseAmount, valrtrade.BaseAmount)

					log.Printf(`bitcoinprofitpercent: %+[1]v`, bitcoinprofitpercent)

					if bitcoinprofitpercent >= profitmargin {

						log.Printf(`bitstamptrade: %+[1]v`, bitstamptrade)
						log.Printf(`valrtrade: %+[1]v`, valrtrade)

						if executetrade {

							var bitstamporder BitstampOrder

							if bitstamporder, err = PostBitstampBuyLimitOrder(
								bitstampkey,
								bitstampsecret,
								bitstampcustomer,
								bitstamphost,
								`btcusd`,
								bitstamptrade.BaseAmount,
								bitstamptrade.QuoteAmount,
								false,
								true,
								false,
							); err != nil {

								log.Panic(err)

								return
							}

							log.Printf(`bitstamporder: %+[1]v`, bitstamporder)

							var valrorderid ValrOrderId

							if valrorderid, err = PostValrLimitOrder(
								valrkey,
								valrsecret,
								valrhost,
								ValrLimitOrder{
									Side:            `SELL`,
									Quantity:        strconv.FormatFloat(valrtrade.BaseAmount, 'g', 1, 64),
									Price:           strconv.FormatFloat(valrtrade.QuoteAmount, 'f', 2, 64),
									Pair:            `BTCZAR`,
									PostOnly:        `False`,
									CustomerOrderId: `1234567890`,
									TimeInForce:     `IOC`,
								},
							); err != nil {

								log.Panic(err)

								return
							}

							log.Printf(`valrorderid: %+[1]v`, valrorderid)

							var bitstamporderstatus BitstampOrderStatus

							if bitstamporderstatus, err = PostBitstampOrderStatus(
								bitstampkey,
								bitstampsecret,
								bitstampcustomer,
								bitstamphost,
								bitstamporder.Id,
							); err != nil {

								log.Panic(err)

								return
							}

							log.Printf(`bitstamporderstatus: %+[1]v`, bitstamporderstatus)

							var valrorderstatus ValrOrderStatus

							if valrorderstatus, err = GetValrOrderStatus(
								valrkey,
								valrsecret,
								valrhost,
								`btczar`,
								valrorderid.Id,
							); err != nil {

								log.Panic(err)

								return
							}

							log.Printf(`valrorderstatus: %+[1]v`, valrorderstatus)
						}
					}
				}
			}
		}

		// time.Sleep(time.Second * 3)

		os.Exit(0)
	}
}

func ReadCsv(filename string) (csvlines [][]string, err error) {

	csvlines = [][]string{}

	var file *os.File

	if file, err = os.Open(filename); err != nil {

		log.Panic(err)

		return
	}

	defer file.Close()

	if csvlines, err = csv.NewReader(file).ReadAll(); err != nil {

		log.Panic(err)

		return
	}

	return
}

func CalculateProfit(buybitcoinvalue float64, sellbitcoinvalue float64) (bitcoinprofitpercent float64) {

	bitcoinprofitpercent = 0.0
	bitcoinprofitpercent += buybitcoinvalue
	bitcoinprofitpercent -= sellbitcoinvalue
	bitcoinprofitpercent /= buybitcoinvalue

	return
}

func CalculateTrade(depth Depth, notional float64) (trade Trade) {

	trade = Trade{}

	var level int = 0
	var levels int = len(depth.Levels)

	var depthlevel Level
	var lastlevel Level

	for level = 0; level < levels; level += 1 {

		if level == 0 {

			lastlevel = Level{}

		} else {

			lastlevel = depthlevel
		}

		depthlevel = depth.Levels[level]

		if depthlevel.NotionalAmount == 0.0 {

			depthlevel.NotionalAmount = 0.0

			depthlevel.NotionalAmount += depthlevel.BaseAmount
			depthlevel.NotionalAmount *= depthlevel.QuoteAmount

			depthlevel.BaseTotal = 0.0

			depthlevel.BaseTotal += lastlevel.BaseTotal
			depthlevel.BaseTotal += depthlevel.BaseAmount

			depthlevel.NotionalTotal = 0.0

			depthlevel.NotionalTotal += lastlevel.NotionalTotal
			depthlevel.NotionalTotal += depthlevel.NotionalAmount

			depthlevel.BaseAhead = 0.0
			depthlevel.BaseAhead += lastlevel.BaseTotal

			depthlevel.NotionalAhead = 0.0
			depthlevel.NotionalAhead += lastlevel.NotionalTotal
		}

		var notionallimitexceeded bool = notional > 0.0 && depthlevel.NotionalTotal > notional

		if notionallimitexceeded {

			break
		}
	}

	if level > -1 && level < levels {

		trade.BaseAmount = depthlevel.BaseTotal
		trade.QuoteAmount = depthlevel.QuoteAmount
		trade.NotionalAmount = depthlevel.NotionalTotal

		var notionallimitexceeded bool = notional > 0.0 && depthlevel.NotionalTotal > notional

		if notionallimitexceeded {

			var notionallimitpercent float64 = 0.0

			notionallimitpercent = 0.0
			notionallimitpercent -= depthlevel.NotionalTotal
			notionallimitpercent += notional
			notionallimitpercent += depthlevel.NotionalAmount
			notionallimitpercent /= depthlevel.NotionalAmount

			trade.BaseAmount = 0.0
			trade.BaseAmount += depthlevel.BaseAmount
			trade.BaseAmount *= notionallimitpercent
			trade.BaseAmount += depthlevel.BaseAhead

			trade.NotionalAmount = notional
		}
	}

	return
}

func GetBitstampDollarBalance(bitstampkey string, bitstampsecret string, bitstampcustomer string, bitstamphost string) (bitstampdollarbalance float64) {

	bitstampdollarbalance = 0.0

	var err error

	var bitstampbalance BitstampBalance

	if bitstampbalance, err = PostBitstampAccountBalance(bitstampkey, bitstampsecret, bitstampcustomer, bitstamphost, `usd`); err != nil {

		log.Panic(err)

		return
	}

	if bitstampdollarbalance, err = strconv.ParseFloat(bitstampbalance.Available, 64); err != nil {

		log.Panic(err)

		return
	}

	return
}

func GetValrBitcoinBalance(valrkey string, valrsecret string, valrhost string) (valrbitcoinbalance float64) {

	valrbitcoinbalance = 0.0

	var err error

	var valrbalancelist []ValrBalance

	if valrbalancelist, err = GetValrBalanceList(valrkey, valrsecret, valrhost); err != nil {

		log.Panic(err)

		return
	}

	var valrbalanceindex int = 0
	var valrbalancelength int = len(valrbalancelist)

	for valrbalanceindex = 0; valrbalanceindex < valrbalancelength; valrbalanceindex++ {

		var valrbalance ValrBalance = valrbalancelist[valrbalanceindex]

		if valrbalance.Currency == `BTC` {

			if valrbitcoinbalance, err = strconv.ParseFloat(valrbalance.Available, 64); err != nil {

				log.Panic(err)

				return
			}
		}
	}

	return
}

func GetBitstampBuyableLiquidity(bitstampkey string, bitstampsecret string, bitstampcustomer string, bitstamphost string) (bitstampbuyable Depth) {

	bitstampbuyable = Depth{
		Type:          Ask,
		BaseCurrency:  `btc`,
		QuoteCurrency: `usd`,
		Levels:        []Level{},
	}

	var err error

	var bitstamporderbook BitstampOrderBook

	if bitstamporderbook, err = GetBitstampOrderBook(bitstampkey, bitstampsecret, bitstampcustomer, bitstamphost, `btcusd`); err != nil {

		log.Panic(err)

		return
	}

	var askindex int = 0
	var asklength int = len(bitstamporderbook.Asks)

	for askindex = 0; askindex < asklength; askindex++ {

		var buylevel Level = Level{}

		if buylevel.BaseAmount, err = strconv.ParseFloat(bitstamporderbook.Asks[askindex][1], 64); err != nil {

			log.Panic(err)

			return
		}

		if buylevel.QuoteAmount, err = strconv.ParseFloat(bitstamporderbook.Asks[askindex][0], 64); err != nil {

			log.Panic(err)

			return
		}

		bitstampbuyable.Levels = append(bitstampbuyable.Levels, buylevel)
	}

	return
}

func GetValrSellableLiquidity(valrkey string, valrsecret string, valrhost string) (valrsellable Depth) {

	valrsellable = Depth{
		Type:          Bid,
		BaseCurrency:  `btc`,
		QuoteCurrency: `zar`,
		Levels:        []Level{},
	}

	var err error

	var valrorderbook ValrOrderBook

	if valrorderbook, err = GetValrOrderBook(valrkey, valrsecret, valrhost, `btczar`); err != nil {

		log.Panic(err)

		return
	}

	var bidindex int = 0
	var bidlength int = len(valrorderbook.Bids)

	for bidindex = 0; bidindex < bidlength; bidindex++ {

		var selllevel Level = Level{}

		if selllevel.BaseAmount, err = strconv.ParseFloat(valrorderbook.Bids[bidindex].Quantity, 64); err != nil {

			log.Panic(err)

			return
		}

		if selllevel.QuoteAmount, err = strconv.ParseFloat(valrorderbook.Bids[bidindex].Price, 64); err != nil {

			log.Panic(err)

			return
		}

		valrsellable.Levels = append(valrsellable.Levels, selllevel)
	}

	return
}

func GetBitstampOrderBook(bitstampkey string, bitstampsecret string, bitstampcustomer string, bitstamphost string, currencypair string) (bitstamporderbook BitstampOrderBook, err error) {

	var urlvalues url.Values = url.Values{
		`group`: []string{strconv.FormatInt(1, 10)},
	}

	var bitstampresponse BitstampResponse = BitstampApi(BitstampRequest{
		Key:      bitstampkey,
		Secret:   bitstampsecret,
		Customer: bitstampcustomer,
		Host:     bitstamphost,
		Method:   http.MethodGet,
		Path:     strings.Join([]string{``, `api`, `v2`, `order_book`, currencypair, ``}, `/`),
		Query:    strings.Join([]string{`?`, urlvalues.Encode()}, ``),
	})

	if bitstampresponse.Error != `` {

		err = errors.New(bitstampresponse.Error)
	}

	if bitstampresponse.Value != `` {

		json.NewDecoder(bytes.NewBufferString(bitstampresponse.Value)).Decode(&bitstamporderbook)
	}

	return
}

func PostBitstampAccountBalance(bitstampkey string, bitstampsecret string, bitstampcustomer string, bitstamphost string, currencypair string) (bitstampbalance BitstampBalance, err error) {

	var bitstampresponse BitstampResponse = BitstampApi(BitstampRequest{
		Key:      bitstampkey,
		Secret:   bitstampsecret,
		Customer: bitstampcustomer,
		Host:     bitstamphost,
		Method:   http.MethodPost,
		Path:     strings.Join([]string{``, `api`, `v2`, `account_balances`, currencypair, ``}, `/`),
	})

	if bitstampresponse.Error != `` {

		err = errors.New(bitstampresponse.Error)
	}

	if bitstampresponse.Value != `` {

		json.NewDecoder(bytes.NewBufferString(bitstampresponse.Value)).Decode(&bitstampbalance)
	}

	return
}

func PostBitstampAccountBalances(bitstampkey string, bitstampsecret string, bitstampcustomer string, bitstamphost string) (bitstampbalance BitstampBalance, err error) {

	var bitstampresponse BitstampResponse = BitstampApi(BitstampRequest{
		Key:      bitstampkey,
		Secret:   bitstampsecret,
		Customer: bitstampcustomer,
		Host:     bitstamphost,
		Method:   http.MethodPost,
		Path:     strings.Join([]string{``, `api`, `v2`, `account_balances`, ``}, `/`),
	})

	if bitstampresponse.Error != `` {

		err = errors.New(bitstampresponse.Error)
	}

	if bitstampresponse.Value != `` {

		json.NewDecoder(bytes.NewBufferString(bitstampresponse.Value)).Decode(&bitstampbalance)
	}

	return
}

func PostBitstampBuyLimitOrder(bitstampkey string, bitstampsecret string, bitstampcustomer string, bitstamphost string, currencypair string, amount float64, price float64, day bool, ioc bool, fok bool) (bitstamporder BitstampOrder, err error) {

	var requestvalues url.Values = url.Values{
		`amount`:      []string{strconv.FormatFloat(amount, 'g', 1, 64)},
		`price`:       []string{strconv.FormatFloat(price, 'f', 2, 64)},
		`daily_order`: []string{strconv.FormatBool(day)},
		`ioc_order`:   []string{strconv.FormatBool(ioc)},
		`fok_order`:   []string{strconv.FormatBool(fok)},
	}

	var bitstampresponse BitstampResponse = BitstampApi(BitstampRequest{
		Key:      bitstampkey,
		Secret:   bitstampsecret,
		Customer: bitstampcustomer,
		Host:     bitstamphost,
		Method:   http.MethodPost,
		Path:     strings.Join([]string{``, `api`, `v2`, `buy`, currencypair, ``}, `/`),
		Request:  requestvalues.Encode(),
		Type:     `application/x-www-form-urlencoded`,
	})

	if bitstampresponse.Error != `` {

		err = errors.New(bitstampresponse.Error)
	}

	if bitstampresponse.Value != `` {

		json.NewDecoder(bytes.NewBufferString(bitstampresponse.Value)).Decode(&bitstamporder)
	}

	return
}

func PostBitstampOrderStatus(bitstampkey string, bitstampsecret string, bitstampcustomer string, bitstamphost string, id string) (bitstamporderstatus BitstampOrderStatus, err error) {

	var requestvalues url.Values = url.Values{
		`id`: []string{id},
	}

	var bitstampresponse BitstampResponse = BitstampApi(BitstampRequest{
		Key:      bitstampkey,
		Secret:   bitstampsecret,
		Customer: bitstampcustomer,
		Host:     bitstamphost,
		Method:   http.MethodPost,
		Path:     strings.Join([]string{``, `api`, `v2`, `order_status`, ``}, `/`),
		Request:  requestvalues.Encode(),
		Type:     `application/x-www-form-urlencoded`,
	})

	if bitstampresponse.Error != `` {

		err = errors.New(bitstampresponse.Error)
	}

	if bitstampresponse.Value != `` {

		json.NewDecoder(bytes.NewBufferString(bitstampresponse.Value)).Decode(&bitstamporderstatus)
	}

	return
}

func GetValrBalanceList(valrkey string, valrsecret string, valrhost string) (valrbalancelist []ValrBalance, err error) {

	var valrresponse ValrResponse = ValrApi(ValrRequest{
		Key:    valrkey,
		Secret: valrsecret,
		Host:   valrhost,
		Method: http.MethodGet,
		Path:   strings.Join([]string{``, `v1`, `account`, `balances`}, `/`),
	})

	if valrresponse.Error != `` {

		err = errors.New(valrresponse.Error)
	}

	if valrresponse.Value != `` {

		json.NewDecoder(bytes.NewBufferString(valrresponse.Value)).Decode(&valrbalancelist)
	}

	return
}

func GetValrOrderBook(valrkey string, valrsecret string, valrhost string, currencypair string) (valrorderbook ValrOrderBook, err error) {

	var valrresponse ValrResponse = ValrApi(ValrRequest{
		Key:    valrkey,
		Secret: valrsecret,
		Host:   valrhost,
		Method: http.MethodGet,
		Path:   strings.Join([]string{``, `v1`, `marketdata`, currencypair, `orderbook`}, `/`),
	})

	if valrresponse.Error != `` {

		err = errors.New(valrresponse.Error)
	}

	if valrresponse.Value != `` {

		json.NewDecoder(bytes.NewBufferString(valrresponse.Value)).Decode(&valrorderbook)
	}

	return
}

func GetValrOrderStatus(valrkey string, valrsecret string, valrhost string, currencypair string, orderid string) (valrorderstatus ValrOrderStatus, err error) {

	var valrresponse ValrResponse = ValrApi(ValrRequest{
		Key:    valrkey,
		Secret: valrsecret,
		Host:   valrhost,
		Method: http.MethodGet,
		Path:   strings.Join([]string{``, `v1`, `orders`, currencypair, `orderid`, orderid}, `/`),
	})

	if valrresponse.Error != `` {

		err = errors.New(valrresponse.Error)
	}

	if valrresponse.Value != `` {

		json.NewDecoder(bytes.NewBufferString(valrresponse.Value)).Decode(&valrorderstatus)
	}

	return
}

func PostValrLimitOrder(valrkey string, valrsecret string, valrhost string, valrlimitorder ValrLimitOrder) (valrorderid ValrOrderId, err error) {

	var requestbuffer *bytes.Buffer = bytes.NewBuffer([]byte{})

	json.NewEncoder(requestbuffer).Encode(valrlimitorder)

	var valrresponse ValrResponse = ValrApi(ValrRequest{
		Key:     valrkey,
		Secret:  valrsecret,
		Host:    valrhost,
		Method:  http.MethodPost,
		Path:    strings.Join([]string{``, `v1`, `orders`, `limit`}, `/`),
		Request: requestbuffer.String(),
		Type:    `application/json`,
	})

	if valrresponse.Error != `` {

		err = errors.New(valrresponse.Error)
	}

	if valrresponse.Value != `` {

		json.NewDecoder(bytes.NewBufferString(valrresponse.Value)).Decode(&valrorderid)
	}

	return
}

func BitstampApi(bitstamprequest BitstampRequest) (bitstampresponse BitstampResponse) {

	var err error

	var requestbuffer *bytes.Buffer = bytes.NewBufferString(bitstamprequest.Request)

	var httpendpoint string = strings.ToLower(strings.Join([]string{`https://`, bitstamprequest.Host, bitstamprequest.Path, bitstamprequest.Query}, ``))

	var httprequest *http.Request

	if httprequest, err = http.NewRequest(bitstamprequest.Method, httpendpoint, requestbuffer); err != nil {

		bitstampresponse.Error = err.Error()

		log.Printf(`Error('%+[1]v')`, bitstampresponse.Error)

		return
	}

	httprequest.Header.Set(`Accept`, `application/json`)

	if bitstamprequest.Type != `` {

		httprequest.Header.Set(`Content-Type`, bitstamprequest.Type)
	}

	var authorisation string = strings.Join([]string{`BITSTAMP`, bitstamprequest.Key}, ` `)

	var timestamp string = strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)

	var uuid []byte = make([]byte, 16)

	rand.Reader.Read(uuid)

	uuid[8] = uuid[8]&^0xc0 | 0x80
	uuid[6] = uuid[6]&^0xf0 | 0x40

	var nonce string = strings.Join([]string{
		hex.EncodeToString(uuid[0:4]),
		hex.EncodeToString(uuid[4:6]),
		hex.EncodeToString(uuid[6:8]),
		hex.EncodeToString(uuid[8:10]),
		hex.EncodeToString(uuid[10:])}, `-`)

	var version string = `v1`

	if strings.Contains(bitstamprequest.Path, `/v2/`) {

		version = `v2`
	}

	var hash hash.Hash = hmac.New(sha256.New, []byte(bitstamprequest.Secret))

	hash.Write([]byte(authorisation))
	hash.Write([]byte(bitstamprequest.Method))
	hash.Write([]byte(bitstamprequest.Host))
	hash.Write([]byte(bitstamprequest.Path))
	hash.Write([]byte(bitstamprequest.Query))
	hash.Write([]byte(bitstamprequest.Type))
	hash.Write([]byte(nonce))
	hash.Write([]byte(timestamp))
	hash.Write([]byte(version))
	hash.Write(requestbuffer.Bytes())

	var signature string = strings.ToUpper(hex.EncodeToString(hash.Sum(nil)))

	if version == `v2` {

		httprequest.Header.Set(`X-Auth`, authorisation)
		httprequest.Header.Set(`X-Auth-Signature`, signature)
		httprequest.Header.Set(`X-Auth-Nonce`, nonce)
		httprequest.Header.Set(`X-Auth-Timestamp`, timestamp)
		httprequest.Header.Set(`X-Auth-Version`, version)
	}

	//log.Printf(`httprequest:%+[1]v`, httprequest)

	var httpclient *http.Client = new(http.Client)

	var httpresponse *http.Response

	if httpresponse, err = httpclient.Do(httprequest); err != nil {

		bitstampresponse.Error = err.Error()

		log.Printf(`Error('%+[1]v')`, bitstampresponse.Error)

		return
	}

	//log.Printf(`httpresponse: %+[1]v`, httpresponse)

	defer httpresponse.Body.Close()

	var responsebuffer *bytes.Buffer = new(bytes.Buffer)

	responsebuffer.ReadFrom(httpresponse.Body)

	//log.Printf(`responsebuffer: %+[1]v`, responsebuffer.String())

	if httpresponse.StatusCode != 200 && httpresponse.StatusCode != 202 {

		bitstampresponse.Error = responsebuffer.String()

		log.Printf(`Error('%+[1]v')`, bitstampresponse.Error)

		return
	}

	bitstampresponse.Value = responsebuffer.String()

	return
}

func ValrApi(valrrequest ValrRequest) (valrresponse ValrResponse) {

	var err error

	var requestbuffer *bytes.Buffer = bytes.NewBufferString(valrrequest.Request)

	var httpendpoint string = strings.ToLower(strings.Join([]string{`https://`, valrrequest.Host, valrrequest.Path, valrrequest.Query}, ``))

	var httprequest *http.Request

	if httprequest, err = http.NewRequest(valrrequest.Method, httpendpoint, requestbuffer); err != nil {

		valrresponse.Error = err.Error()

		log.Printf(`Error('%+[1]v')`, valrresponse.Error)

		return
	}

	httprequest.Header.Set(`Accept`, `application/json`)

	if valrrequest.Type != `` {

		httprequest.Header.Set(`Content-Type`, valrrequest.Type)
	}

	var timestamp string = strconv.FormatInt(time.Now().UnixNano()/int64(time.Millisecond), 10)

	var hash hash.Hash = hmac.New(sha512.New, []byte(valrrequest.Secret))

	hash.Write([]byte(timestamp))
	hash.Write([]byte(valrrequest.Method))
	hash.Write([]byte(valrrequest.Path))
	hash.Write(requestbuffer.Bytes())

	var signature string = hex.EncodeToString(hash.Sum(nil))

	httprequest.Header.Set(`X-VALR-API-KEY`, valrrequest.Key)
	httprequest.Header.Set(`X-VALR-SIGNATURE`, signature)
	httprequest.Header.Set(`X-VALR-TIMESTAMP`, timestamp)

	//log.Printf(`httprequest:%+[1]v`, httprequest)

	var httpclient *http.Client = new(http.Client)

	var httpresponse *http.Response

	if httpresponse, err = httpclient.Do(httprequest); err != nil {

		valrresponse.Error = err.Error()

		log.Printf(`Error('%+[1]v')`, valrresponse.Error)

		return
	}

	//log.Printf(`httpresponse: %+[1]v`, httpresponse)

	defer httpresponse.Body.Close()

	var responsebuffer *bytes.Buffer = new(bytes.Buffer)

	responsebuffer.ReadFrom(httpresponse.Body)

	//log.Printf(`responsebuffer: %+[1]v`, responsebuffer.String())

	if httpresponse.StatusCode != 200 && httpresponse.StatusCode != 202 {

		valrresponse.Error = responsebuffer.String()

		log.Printf(`Error('%+[1]v')`, valrresponse.Error)

		return
	}

	valrresponse.Value = responsebuffer.String()

	return
}

const (
	Undefined = 0
	Bid       = 1
	Ask       = 2
)

type Depth struct {
	Type          int
	BaseCurrency  string
	QuoteCurrency string
	Levels        []Level
}

type Level struct {
	BaseAmount     float64
	BaseAhead      float64
	BaseTotal      float64
	QuoteAmount    float64
	NotionalAmount float64
	NotionalAhead  float64
	NotionalTotal  float64
}

type Trade struct {
	BaseAmount     float64
	QuoteAmount    float64
	NotionalAmount float64
}

type BitstampRequest struct {
	Key      string
	Secret   string
	Customer string
	Host     string
	Method   string
	Path     string
	Query    string
	Request  string
	Type     string
}

type BitstampResponse struct {
	Value string
	Error string
}

type ValrRequest struct {
	Key     string
	Secret  string
	Host    string
	Method  string
	Path    string
	Query   string
	Request string
	Type    string
}

type ValrResponse struct {
	Value string
	Error string
}

type BitstampBalance struct {
	Currency  string `json:"currency"`
	Total     string `json:"total"`
	Available string `json:"available"`
	Reserved  string `json:"reserved"`
}

type BitstampOrder struct {
	Id            string `json:"id"`
	DateTime      string `json:"datetime"`
	Type          string `json:"type"`
	Price         string `json:"price"`
	Amount        string `json:"amount"`
	ClientOrderId string `json:"client_order_id"`
}

type BitstampOrderBook struct {
	Timestamp      string     `json:"timestamp"`
	Microtimestamp string     `json:"microtimestamp"`
	Bids           [][]string `json:"bids"`
	Asks           [][]string `json:"asks"`
}

type BitstampOrderStatus struct {
	Status          string                `json:"status"`
	Id              string                `json:"id"`
	Transactions    []BitstampTransaction `json:"transactions"`
	AmountRemaining string                `json:"amount_remaining"`
	ClientOrderId   string                `json:"client_order_id"`
}

type BitstampTransaction struct {
	Tid      string `json:"tid"`
	Usd      string `json:"usd"`
	Price    string `json:"price"`
	Fee      string `json:"fee"`
	Btc      string `json:"btc"`
	DateTime string `json:"datetime"`
	Type     string `json:"type"`
}

type ValrBalance struct {
	Currency  string `json:"currency"`
	Available string `json:"available"`
	Reserved  string `json:"reserved"`
	Total     string `json:"total"`
	UpdatedAt string `json:"updatedAt"`
}

type ValrLimitOrder struct {
	Side            string `json:"side"`
	Quantity        string `json:"quantity"`
	Price           string `json:"price"`
	Pair            string `json:"pair"`
	PostOnly        string `json:"postOnly"`
	CustomerOrderId string `json:"customerOrderId"`
	TimeInForce     string `json:"timeInForce"`
}

type ValrOrder struct {
	Side         string `json:"side"`
	Quantity     string `json:"quantity"`
	Price        string `json:"price"`
	CurrencyPair string `json:"currencyPair"`
	OrderCount   int    `json:"orderCount"`
}

type ValrOrderBook struct {
	Bids       []ValrOrder `json:"Bids"`
	Asks       []ValrOrder `json:"Asks"`
	LastChange string      `json:"LastChange"`
}

type ValrOrderId struct {
	Id string `json:"id"`
}

type ValrOrderStatus struct {
	OrderId           string `json:"orderId"`
	OrderStatusType   string `json:"orderStatusType"`
	CurrencyPair      string `json:"currencyPair"`
	OriginalPrice     string `json:"originalPrice"`
	RemainingQuantity string `json:"remainingQuantity"`
	OriginalQuantity  string `json:"originalQuantity"`
	OrderSide         string `json:"orderSide"`
	OrderType         string `json:"orderType"`
	FailedReason      string `json:"failedReason"`
	CustomerOrderId   string `json:"customerOrderId"`
	OrderUpdatedAt    string `json:"orderUpdatedAt"`
	OrderCreatedAt    string `json:"orderCreatedAt"`
	TimeInForce       string `json:"timeInForce"`
}
