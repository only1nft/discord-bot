package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type CoinGeckoTickerData struct {
	Usd             float64 `json:"usd"`
	UsdMarketCap    float64 `json:"usd_market_cap"`
	Usd24hVolume    float64 `json:"usd_24h_vol"`
	Usd24hChangePct float64 `json:"usd_24h_change"`
}

type CoinGeckoResponse struct {
	Only1 CoinGeckoTickerData `json:"only1"`
}

func getPriceDataWithRetries(attempt uint8) (*CoinGeckoTickerData, error) {
	const maxAttempts uint8 = 5

	if attempt > 0 {
		time.Sleep(time.Second * 10 * time.Duration(attempt))
	}

	resp, err := http.Get("https://api.coingecko.com/api/v3/simple/price?ids=only1&vs_currencies=usd&include_market_cap=true&include_24hr_vol=true&include_24hr_change=true")
	if err != nil {
		log.Printf("failed to retrieve coingecko price: %v", err)
		if attempt == maxAttempts {
			return nil, err
		}
		return getPriceDataWithRetries(attempt + 1)
	}

	var parsed CoinGeckoResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		log.Printf("failed to parse coingecko response: %v", err)
		if attempt == maxAttempts {
			return nil, err
		}
		return getPriceDataWithRetries(attempt + 1)
	}

	return &parsed.Only1, nil
}

func GetPriceData() (*CoinGeckoTickerData, error) {
	return getPriceDataWithRetries(0)
}
