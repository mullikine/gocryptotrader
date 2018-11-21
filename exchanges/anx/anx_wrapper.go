package anx

import (
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/thrasher-/gocryptotrader/common"
	"github.com/thrasher-/gocryptotrader/config"
	"github.com/thrasher-/gocryptotrader/currency/pair"
	"github.com/thrasher-/gocryptotrader/exchanges"
	"github.com/thrasher-/gocryptotrader/exchanges/assets"
	"github.com/thrasher-/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-/gocryptotrader/exchanges/request"
	"github.com/thrasher-/gocryptotrader/exchanges/ticker"
)

// SetDefaults sets current default settings
func (a *ANX) SetDefaults() {
	a.Name = "ANX"
	a.Enabled = true
	a.Verbose = true
	a.RequestCurrencyPairFormat.Uppercase = true
	a.ConfigCurrencyPairFormat.Delimiter = "_"
	a.ConfigCurrencyPairFormat.Uppercase = true
	a.APIWithdrawPermissions = exchange.WithdrawCryptoWithEmail | exchange.AutoWithdrawCryptoWithSetup |
		exchange.WithdrawCryptoWith2FA | exchange.WithdrawFiatViaWebsiteOnly
	a.AssetTypes = assets.AssetTypes{assets.AssetTypeSpot}
	a.Features = exchange.Features{
		Supports: exchange.FeaturesSupported{
			AutoPairUpdates:    true,
			RESTTickerBatching: false,
			REST:               true,
			Websocket:          false,
		},
		Enabled: exchange.FeaturesEnabled{
			AutoPairUpdates: true,
		},
	}
	a.Requester = request.New(a.Name,
		request.NewRateLimit(time.Second, anxAuthRate),
		request.NewRateLimit(time.Second, anxUnauthRate),
		common.NewHTTPClientWithTimeout(exchange.DefaultHTTPTimeout))
	a.API.Endpoints.URLDefault = anxAPIURL
	a.API.Endpoints.URL = a.API.Endpoints.URLDefault
}

//Setup is run on startup to setup exchange with config values
func (a *ANX) Setup(exch config.ExchangeConfig) error {
	if !exch.Enabled {
		a.SetEnabled(false)
		return nil
	}

	return a.SetupDefaults(exch)
}

// Start starts the ANX go routine
func (a *ANX) Start(wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		a.Run()
		wg.Done()
	}()
}

// Run implements the ANX wrapper
func (a *ANX) Run() {
	if a.Verbose {
		log.Printf("%s %d currencies enabled: %s.\n", a.GetName(), len(a.EnabledPairs), a.EnabledPairs)
	}

	forceUpdate := false
	if !common.StringDataContains(a.EnabledPairs, "_") || !common.StringDataContains(a.AvailablePairs, "_") {
		enabledPairs := []string{"BTC_USD,BTC_HKD,BTC_EUR,BTC_CAD,BTC_AUD,BTC_SGD,BTC_JPY,BTC_GBP,BTC_NZD,LTC_BTC,DOG_EBTC,STR_BTC,XRP_BTC"}
		log.Println("WARNING: Enabled pairs for ANX reset due to config upgrade, please enable the ones you would like again.")

		forceUpdate = true
		err := a.UpdatePairs(enabledPairs, true, true)
		if err != nil {
			log.Printf("%s failed to update currencies.\n", a.GetName())
			return
		}
	}

	if !a.GetEnabledFeatures().AutoPairUpdates && !forceUpdate {
		return
	}

	err := a.UpdateTradablePairs(forceUpdate)
	if err != nil {
		log.Printf("%s failed to update tradable pairs. Err: %s", a.GetName(), err)
	}
}

// UpdateTradablePairs updates the exchanges available pairs and stores
// them in the exchanges config
func (a *ANX) UpdateTradablePairs(forceUpdate bool) error {
	pairs, err := a.FetchTradablePairs()
	if err != nil {
		return err
	}

	return a.UpdatePairs(pairs, false, forceUpdate)
}

// FetchTradablePairs returns a list of the exchanges tradable pairs
func (a *ANX) FetchTradablePairs() ([]string, error) {
	result, err := a.GetCurrencies()
	if err != nil {
		return nil, err
	}

	var currencies []string
	for x := range result.CurrencyPairs {
		currencies = append(currencies, result.CurrencyPairs[x].TradedCcy+"_"+result.CurrencyPairs[x].SettlementCcy)
	}

	return currencies, nil
}

// UpdateTicker updates and returns the ticker for a currency pair
func (a *ANX) UpdateTicker(p pair.CurrencyPair, assetType assets.AssetType) (ticker.Price, error) {
	var tickerPrice ticker.Price
	tick, err := a.GetTicker(exchange.FormatExchangeCurrency(a.GetName(), p).String())
	if err != nil {
		return tickerPrice, err
	}

	tickerPrice.Pair = p

	if tick.Data.Sell.Value != "" {
		tickerPrice.Ask, err = strconv.ParseFloat(tick.Data.Sell.Value, 64)
		if err != nil {
			return tickerPrice, err
		}
	} else {
		tickerPrice.Ask = 0
	}

	if tick.Data.Buy.Value != "" {
		tickerPrice.Bid, err = strconv.ParseFloat(tick.Data.Buy.Value, 64)
		if err != nil {
			return tickerPrice, err
		}
	} else {
		tickerPrice.Bid = 0
	}

	if tick.Data.Low.Value != "" {
		tickerPrice.Low, err = strconv.ParseFloat(tick.Data.Low.Value, 64)
		if err != nil {
			return tickerPrice, err
		}
	} else {
		tickerPrice.Low = 0
	}

	if tick.Data.Last.Value != "" {
		tickerPrice.Last, err = strconv.ParseFloat(tick.Data.Last.Value, 64)
		if err != nil {
			return tickerPrice, err
		}
	} else {
		tickerPrice.Last = 0
	}

	if tick.Data.Vol.Value != "" {
		tickerPrice.Volume, err = strconv.ParseFloat(tick.Data.Vol.Value, 64)
		if err != nil {
			return tickerPrice, err
		}
	} else {
		tickerPrice.Volume = 0
	}

	if tick.Data.High.Value != "" {
		tickerPrice.High, err = strconv.ParseFloat(tick.Data.High.Value, 64)
		if err != nil {
			return tickerPrice, err
		}
	} else {
		tickerPrice.High = 0
	}
	ticker.ProcessTicker(a.GetName(), p, tickerPrice, assetType)
	return ticker.GetTicker(a.Name, p, assetType)
}

// FetchTicker returns the ticker for a currency pair
func (a *ANX) FetchTicker(p pair.CurrencyPair, assetType assets.AssetType) (ticker.Price, error) {
	tickerNew, err := ticker.GetTicker(a.GetName(), p, assetType)
	if err != nil {
		return a.UpdateTicker(p, assetType)
	}
	return tickerNew, nil
}

// FetchOrderbook returns the orderbook for a currency pair
func (a *ANX) FetchOrderbook(p pair.CurrencyPair, assetType assets.AssetType) (orderbook.Base, error) {
	ob, err := orderbook.GetOrderbook(a.GetName(), p, assetType)
	if err != nil {
		return a.UpdateOrderbook(p, assetType)
	}
	return ob, nil
}

// UpdateOrderbook updates and returns the orderbook for a currency pair
func (a *ANX) UpdateOrderbook(p pair.CurrencyPair, assetType assets.AssetType) (orderbook.Base, error) {
	var orderBook orderbook.Base
	orderbookNew, err := a.GetDepth(exchange.FormatExchangeCurrency(a.GetName(), p).String())
	if err != nil {
		return orderBook, err
	}

	for x := range orderbookNew.Data.Asks {
		orderBook.Asks = append(orderBook.Asks, orderbook.Item{Price: orderbookNew.Data.Asks[x].Price, Amount: orderbookNew.Data.Asks[x].Amount})
	}

	for x := range orderbookNew.Data.Bids {
		orderBook.Bids = append(orderBook.Bids, orderbook.Item{Price: orderbookNew.Data.Bids[x].Price, Amount: orderbookNew.Data.Bids[x].Amount})
	}

	orderbook.ProcessOrderbook(a.GetName(), p, orderBook, assetType)
	return orderbook.GetOrderbook(a.Name, p, assetType)
}

// GetAccountInfo retrieves balances for all enabled currencies on the
// exchange
func (a *ANX) GetAccountInfo() (exchange.AccountInfo, error) {
	var info exchange.AccountInfo
	return info, common.ErrNotYetImplemented
}

// GetFundingHistory returns funding history, deposits and
// withdrawals
func (a *ANX) GetFundingHistory() ([]exchange.FundHistory, error) {
	var fundHistory []exchange.FundHistory
	return fundHistory, common.ErrFunctionNotSupported
}

// GetExchangeHistory returns historic trade data since exchange opening.
func (a *ANX) GetExchangeHistory(p pair.CurrencyPair, assetType assets.AssetType) ([]exchange.TradeHistory, error) {
	var resp []exchange.TradeHistory

	return resp, common.ErrNotYetImplemented
}

// SubmitOrder submits a new order
func (a *ANX) SubmitOrder(p pair.CurrencyPair, side exchange.OrderSide, orderType exchange.OrderType, amount, price float64, clientID string) (exchange.SubmitOrderResponse, error) {
	var submitOrderResponse exchange.SubmitOrderResponse

	var isBuying bool
	var limitPriceInSettlementCurrency float64

	if side == exchange.Buy {
		isBuying = true
	}

	if orderType == exchange.Limit {
		limitPriceInSettlementCurrency = price
	}

	response, err := a.NewOrder(orderType.ToString(), isBuying, p.FirstCurrency.String(), amount, p.SecondCurrency.String(), amount, limitPriceInSettlementCurrency, false, "", false)
	if response != "" {
		submitOrderResponse.OrderID = response
	}

	if err == nil {
		submitOrderResponse.IsOrderPlaced = true
	}

	return submitOrderResponse, err
}

// ModifyOrder will allow of changing orderbook placement and limit to
// market conversion
func (a *ANX) ModifyOrder(orderID int64, action exchange.ModifyOrder) (int64, error) {
	return 0, common.ErrNotYetImplemented
}

// CancelOrder cancels an order by its corresponding ID number
func (a *ANX) CancelOrder(order exchange.OrderCancellation) error {
	orderIDs := []string{order.OrderID}
	return a.CancelOrderByIDs(orderIDs)
}

// CancelAllOrders cancels all orders associated with a currency pair
func (a *ANX) CancelAllOrders() error {
	return common.ErrNotYetImplemented
}

// GetOrderInfo returns information on a current open order
func (a *ANX) GetOrderInfo(orderID int64) (exchange.OrderDetail, error) {
	var orderDetail exchange.OrderDetail
	return orderDetail, common.ErrNotYetImplemented
}

// GetDepositAddress returns a deposit address for a specified currency
func (a *ANX) GetDepositAddress(cryptocurrency pair.CurrencyItem) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawCryptocurrencyFunds returns a withdrawal ID when a withdrawal is
// submitted
func (a *ANX) WithdrawCryptocurrencyFunds(address string, cryptocurrency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawFiatFunds returns a withdrawal ID when a withdrawal is
// submitted
func (a *ANX) WithdrawFiatFunds(currency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawFiatFundsToInternationalBank returns a withdrawal ID when a withdrawal is
// submitted
func (a *ANX) WithdrawFiatFundsToInternationalBank(currency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// GetWebsocket returns a pointer to the exchange websocket
func (a *ANX) GetWebsocket() (*exchange.Websocket, error) {
	return nil, common.ErrFunctionNotSupported
}

// GetFeeByType returns an estimate of fee based on type of transaction
func (a *ANX) GetFeeByType(feeBuilder exchange.FeeBuilder) (float64, error) {
	return a.GetFee(feeBuilder)
}

// GetWithdrawCapabilities returns the types of withdrawal methods permitted by the exchange
func (a *ANX) GetWithdrawCapabilities() uint32 {
	return a.GetWithdrawPermissions()
}
