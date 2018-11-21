package wex

import (
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/thrasher-/gocryptotrader/common"
	"github.com/thrasher-/gocryptotrader/config"
	"github.com/thrasher-/gocryptotrader/currency/pair"
	exchange "github.com/thrasher-/gocryptotrader/exchanges"
	"github.com/thrasher-/gocryptotrader/exchanges/assets"
	"github.com/thrasher-/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-/gocryptotrader/exchanges/request"
	"github.com/thrasher-/gocryptotrader/exchanges/ticker"
)

// SetDefaults sets current default value for WEX
func (w *WEX) SetDefaults() {
	w.Name = "WEX"
	w.Enabled = true
	w.Verbose = true
	w.APIWithdrawPermissions = exchange.AutoWithdrawCryptoWithAPIPermission
	w.RequestCurrencyPairFormat.Delimiter = "_"
	w.RequestCurrencyPairFormat.Separator = "-"
	w.ConfigCurrencyPairFormat.Delimiter = "_"
	w.ConfigCurrencyPairFormat.Uppercase = true
	w.AssetTypes = assets.AssetTypes{assets.AssetTypeSpot}
	w.Features = exchange.Features{
		Supports: exchange.FeaturesSupported{
			AutoPairUpdates:    true,
			RESTTickerBatching: true,
			REST:               true,
			Websocket:          false,
		},
		Enabled: exchange.FeaturesEnabled{
			AutoPairUpdates: true,
		},
	}
	w.Requester = request.New(w.Name,
		request.NewRateLimit(time.Second, wexAuthRate),
		request.NewRateLimit(time.Second, wexUnauthRate),
		common.NewHTTPClientWithTimeout(exchange.DefaultHTTPTimeout))
	w.API.Endpoints.URLDefault = wexAPIPublicURL
	w.API.Endpoints.URL = w.API.Endpoints.URLDefault
	w.API.Endpoints.URLSecondaryDefault = wexAPIPrivateURL
	w.API.Endpoints.URLSecondary = w.API.Endpoints.URLSecondaryDefault
}

// Setup sets exchange configuration parameters for WEX
func (w *WEX) Setup(exch config.ExchangeConfig) error {
	if !exch.Enabled {
		w.SetEnabled(false)
		return nil
	}

	return w.SetupDefaults(exch)
}

// Start starts the WEX go routine
func (w *WEX) Start(wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		w.Run()
		wg.Done()
	}()
}

// Run implements the WEX wrapper
func (w *WEX) Run() {
	if w.Verbose {
		log.Printf("%s %d currencies enabled: %s.\n", w.GetName(), len(w.EnabledPairs), w.EnabledPairs)
	}

	forceUpdate := false
	if !common.StringDataContains(w.EnabledPairs, "_") || !common.StringDataContains(w.AvailablePairs, "_") {
		enabledPairs := []string{"BTC_USD", "LTC_USD", "LTC_BTC", "ETH_USD"}
		log.Println("WARNING: Enabled pairs for WEX reset due to config upgrade, please enable the ones you would like again.")
		forceUpdate = true

		err := w.UpdatePairs(enabledPairs, true, true)
		if err != nil {
			log.Printf("%s failed to update currencies. Err: %s\n", w.Name, err)
		}
	}

	if !w.GetEnabledFeatures().AutoPairUpdates && !forceUpdate {
		return
	}

	err := w.UpdateTradablePairs(forceUpdate)
	if err != nil {
		log.Printf("%s failed to update tradable pairs. Err: %s", w.Name, err)
	}
}

// FetchTradablePairs returns a list of the exchanges tradable pairs
func (w *WEX) FetchTradablePairs() ([]string, error) {
	info, err := w.GetInfo()
	if err != nil {
		return nil, err
	}

	var currencies []string
	for x := range info.Pairs {
		currencies = append(currencies, common.StringToUpper(x))
	}

	return currencies, nil
}

// UpdateTradablePairs updates the exchanges available pairs and stores
// them in the exchanges config
func (w *WEX) UpdateTradablePairs(forceUpdate bool) error {
	pairs, err := w.FetchTradablePairs()
	if err != nil {
		return err
	}

	return w.UpdatePairs(pairs, false, forceUpdate)
}

// UpdateTicker updates and returns the ticker for a currency pair
func (w *WEX) UpdateTicker(p pair.CurrencyPair, assetType assets.AssetType) (ticker.Price, error) {
	var tickerPrice ticker.Price
	pairsCollated, err := exchange.FormatExchangeCurrencies(w.Name, w.GetEnabledPairs())
	if err != nil {
		return tickerPrice, err
	}

	result, err := w.GetTicker(pairsCollated.String())
	if err != nil {
		return tickerPrice, err
	}

	for _, x := range w.GetEnabledPairs() {
		currency := exchange.FormatExchangeCurrency(w.Name, x).Lower().String()
		var tp ticker.Price
		tp.Pair = x
		tp.Last = result[currency].Last
		tp.Ask = result[currency].Sell
		tp.Bid = result[currency].Buy
		tp.Last = result[currency].Last
		tp.Low = result[currency].Low
		tp.Volume = result[currency].VolumeCurrent
		ticker.ProcessTicker(w.Name, x, tp, assetType)
	}
	return ticker.GetTicker(w.Name, p, assetType)
}

// FetchTicker returns the ticker for a currency pair
func (w *WEX) FetchTicker(p pair.CurrencyPair, assetType assets.AssetType) (ticker.Price, error) {
	tick, err := ticker.GetTicker(w.GetName(), p, assetType)
	if err != nil {
		return w.UpdateTicker(p, assetType)
	}
	return tick, nil
}

// FetchOrderbook returns the orderbook for a currency pair
func (w *WEX) FetchOrderbook(p pair.CurrencyPair, assetType assets.AssetType) (orderbook.Base, error) {
	ob, err := orderbook.GetOrderbook(w.GetName(), p, assetType)
	if err != nil {
		return w.UpdateOrderbook(p, assetType)
	}
	return ob, nil
}

// UpdateOrderbook updates and returns the orderbook for a currency pair
func (w *WEX) UpdateOrderbook(p pair.CurrencyPair, assetType assets.AssetType) (orderbook.Base, error) {
	var orderBook orderbook.Base
	orderbookNew, err := w.GetDepth(exchange.FormatExchangeCurrency(w.Name, p).String())
	if err != nil {
		return orderBook, err
	}

	for x := range orderbookNew.Bids {
		data := orderbookNew.Bids[x]
		orderBook.Bids = append(orderBook.Bids, orderbook.Item{Price: data[0], Amount: data[1]})
	}

	for x := range orderbookNew.Asks {
		data := orderbookNew.Asks[x]
		orderBook.Asks = append(orderBook.Asks, orderbook.Item{Price: data[0], Amount: data[1]})
	}

	orderbook.ProcessOrderbook(w.GetName(), p, orderBook, assetType)
	return orderbook.GetOrderbook(w.Name, p, assetType)
}

// GetAccountInfo retrieves balances for all enabled currencies for the
// WEX exchange
func (w *WEX) GetAccountInfo() (exchange.AccountInfo, error) {
	var response exchange.AccountInfo
	response.ExchangeName = w.GetName()
	accountBalance, err := w.GetAccountInformation()
	if err != nil {
		return response, err
	}

	for x, y := range accountBalance.Funds {
		var exchangeCurrency exchange.AccountCurrencyInfo
		exchangeCurrency.CurrencyName = common.StringToUpper(x)
		exchangeCurrency.TotalValue = y
		exchangeCurrency.Hold = 0
		response.Currencies = append(response.Currencies, exchangeCurrency)
	}

	return response, nil
}

// GetFundingHistory returns funding history, deposits and
// withdrawals
func (w *WEX) GetFundingHistory() ([]exchange.FundHistory, error) {
	var fundHistory []exchange.FundHistory
	return fundHistory, common.ErrFunctionNotSupported
}

// GetExchangeHistory returns historic trade data since exchange opening.
func (w *WEX) GetExchangeHistory(p pair.CurrencyPair, assetType assets.AssetType) ([]exchange.TradeHistory, error) {
	var resp []exchange.TradeHistory

	return resp, common.ErrNotYetImplemented
}

// SubmitOrder submits a new order
func (w *WEX) SubmitOrder(p pair.CurrencyPair, side exchange.OrderSide, orderType exchange.OrderType, amount, price float64, clientID string) (exchange.SubmitOrderResponse, error) {
	var submitOrderResponse exchange.SubmitOrderResponse
	response, err := w.Trade(common.StringToLower(p.Pair().String()), common.StringToLower(side.ToString()), amount, price)

	if response > 0 {
		submitOrderResponse.OrderID = fmt.Sprintf("%v", response)
	}

	if err == nil {
		submitOrderResponse.IsOrderPlaced = true
	}

	return submitOrderResponse, err
}

// ModifyOrder will allow of changing orderbook placement and limit to
// market conversion
func (w *WEX) ModifyOrder(orderID int64, action exchange.ModifyOrder) (int64, error) {
	return 0, common.ErrNotYetImplemented
}

// CancelOrder cancels an order by its corresponding ID number
func (w *WEX) CancelOrder(order exchange.OrderCancellation) error {
	orderIDInt, err := strconv.ParseInt(order.OrderID, 10, 64)

	if err != nil {
		return err
	}

	_, err = w.CancelExistingOrder(orderIDInt)

	return err
}

// CancelAllOrders cancels all orders associated with a currency pair
func (w *WEX) CancelAllOrders() error {
	return common.ErrNotYetImplemented
}

// GetOrderInfo returns information on a current open order
func (w *WEX) GetOrderInfo(orderID int64) (exchange.OrderDetail, error) {
	var orderDetail exchange.OrderDetail
	return orderDetail, common.ErrNotYetImplemented
}

// GetDepositAddress returns a deposit address for a specified currency
func (w *WEX) GetDepositAddress(cryptocurrency pair.CurrencyItem) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawCryptocurrencyFunds returns a withdrawal ID when a withdrawal is
// submitted
func (w *WEX) WithdrawCryptocurrencyFunds(address string, cryptocurrency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawFiatFunds returns a withdrawal ID when a
// withdrawal is submitted
func (w *WEX) WithdrawFiatFunds(currency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawFiatFundsToInternationalBank returns a withdrawal ID when a
// withdrawal is submitted
func (w *WEX) WithdrawFiatFundsToInternationalBank(currency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// GetWebsocket returns a pointer to the exchange websocket
func (w *WEX) GetWebsocket() (*exchange.Websocket, error) {
	return nil, common.ErrNotYetImplemented
}

// GetFeeByType returns an estimate of fee based on type of transaction
func (w *WEX) GetFeeByType(feeBuilder exchange.FeeBuilder) (float64, error) {
	return w.GetFee(feeBuilder)
}

// GetWithdrawCapabilities returns the types of withdrawal methods permitted by the exchange
func (w *WEX) GetWithdrawCapabilities() uint32 {
	return w.GetWithdrawPermissions()
}
