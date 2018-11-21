package hitbtc

import (
	"fmt"
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

// SetDefaults sets default settings for hitbtc
func (h *HitBTC) SetDefaults() {
	h.Name = "HitBTC"
	h.Enabled = true
	h.Verbose = true
	h.APIWithdrawPermissions = exchange.AutoWithdrawCrypto
	h.RequestCurrencyPairFormat.Uppercase = true
	h.ConfigCurrencyPairFormat.Delimiter = "-"
	h.ConfigCurrencyPairFormat.Uppercase = true
	h.AssetTypes = assets.AssetTypes{assets.AssetTypeSpot}
	h.Features = exchange.Features{
		Supports: exchange.FeaturesSupported{
			AutoPairUpdates:    true,
			RESTTickerBatching: true,
			REST:               true,
			Websocket:          true,
		},
		Enabled: exchange.FeaturesEnabled{
			AutoPairUpdates: true,
		},
	}
	h.Requester = request.New(h.Name,
		request.NewRateLimit(time.Second, hitbtcAuthRate),
		request.NewRateLimit(time.Second, hitbtcUnauthRate),
		common.NewHTTPClientWithTimeout(exchange.DefaultHTTPTimeout))
	h.API.Endpoints.URLDefault = apiURL
	h.API.Endpoints.URL = h.API.Endpoints.URLDefault
	h.WebsocketInit()
}

// Setup sets user exchange configuration settings
func (h *HitBTC) Setup(exch config.ExchangeConfig) error {
	if !exch.Enabled {
		h.SetEnabled(false)
		return nil
	}

	err := h.SetupDefaults(exch)
	if err != nil {
		return err
	}

	return h.WebsocketSetup(h.WsConnect,
		exch.Name,
		exch.Features.Enabled.Websocket,
		hitbtcWebsocketAddress,
		exch.API.Endpoints.WebsocketURL)
}

// Start starts the HitBTC go routine
func (h *HitBTC) Start(wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		h.Run()
		wg.Done()
	}()
}

// Run implements the HitBTC wrapper
func (h *HitBTC) Run() {
	if h.Verbose {
		log.Printf("%s Websocket: %s (url: %s).\n", h.GetName(), common.IsEnabled(h.Websocket.IsEnabled()), hitbtcWebsocketAddress)
		log.Printf("%s %d currencies enabled: %s.\n", h.GetName(), len(h.EnabledPairs), h.EnabledPairs)
	}

	forceUpdate := false
	if !common.StringDataContains(h.EnabledPairs, "-") || !common.StringDataContains(h.AvailablePairs, "-") {
		enabledPairs := []string{"BTC-USD"}
		log.Println("WARNING: Available pairs for HitBTC reset due to config upgrade, please enable the ones you would like again.")
		forceUpdate = true

		err := h.UpdatePairs(enabledPairs, true, true)
		if err != nil {
			log.Printf("%s failed to update enabled currencies.\n", h.GetName())
		}
	}

	if !h.GetEnabledFeatures().AutoPairUpdates && !forceUpdate {
		return
	}

	err := h.UpdateTradablePairs(forceUpdate)
	if err != nil {
		log.Printf("%s failed to update tradable pairs. Err: %s", h.Name, err)
	}
}

// FetchTradablePairs returns a list of the exchanges tradable pairs
func (h *HitBTC) FetchTradablePairs() ([]string, error) {
	symbols, err := h.GetSymbolsDetailed()
	if err != nil {
		return nil, err
	}

	var pairs []string
	for x := range symbols {
		pairs = append(pairs, symbols[x].BaseCurrency+"-"+symbols[x].QuoteCurrency)
	}
	return pairs, nil
}

// UpdateTradablePairs updates the exchanges available pairs and stores
// them in the exchanges config
func (h *HitBTC) UpdateTradablePairs(forceUpdate bool) error {
	pairs, err := h.FetchTradablePairs()
	if err != nil {
		return err
	}

	return h.UpdatePairs(pairs, false, forceUpdate)
}

// UpdateTicker updates and returns the ticker for a currency pair
func (h *HitBTC) UpdateTicker(currencyPair pair.CurrencyPair, assetType assets.AssetType) (ticker.Price, error) {
	tick, err := h.GetTicker("")
	if err != nil {
		return ticker.Price{}, err
	}

	for _, x := range h.GetEnabledPairs() {
		var tp ticker.Price
		curr := exchange.FormatExchangeCurrency(h.GetName(), x).String()
		tp.Pair = x
		tp.Ask = tick[curr].Ask
		tp.Bid = tick[curr].Bid
		tp.High = tick[curr].High
		tp.Last = tick[curr].Last
		tp.Low = tick[curr].Low
		tp.Volume = tick[curr].Volume
		ticker.ProcessTicker(h.GetName(), x, tp, assetType)
	}
	return ticker.GetTicker(h.Name, currencyPair, assetType)
}

// FetchTicker returns the ticker for a currency pair
func (h *HitBTC) FetchTicker(currencyPair pair.CurrencyPair, assetType assets.AssetType) (ticker.Price, error) {
	tickerNew, err := ticker.GetTicker(h.GetName(), currencyPair, assetType)
	if err != nil {
		return h.UpdateTicker(currencyPair, assetType)
	}
	return tickerNew, nil
}

// FetchOrderbook returns orderbook base on the currency pair
func (h *HitBTC) FetchOrderbook(currencyPair pair.CurrencyPair, assetType assets.AssetType) (orderbook.Base, error) {
	ob, err := orderbook.GetOrderbook(h.GetName(), currencyPair, assetType)
	if err != nil {
		return h.UpdateOrderbook(currencyPair, assetType)
	}
	return ob, nil
}

// UpdateOrderbook updates and returns the orderbook for a currency pair
func (h *HitBTC) UpdateOrderbook(currencyPair pair.CurrencyPair, assetType assets.AssetType) (orderbook.Base, error) {
	var orderBook orderbook.Base
	orderbookNew, err := h.GetOrderbook(exchange.FormatExchangeCurrency(h.GetName(), currencyPair).String(), 1000)
	if err != nil {
		return orderBook, err
	}

	for x := range orderbookNew.Bids {
		data := orderbookNew.Bids[x]
		orderBook.Bids = append(orderBook.Bids, orderbook.Item{Amount: data.Amount, Price: data.Price})
	}

	for x := range orderbookNew.Asks {
		data := orderbookNew.Asks[x]
		orderBook.Asks = append(orderBook.Asks, orderbook.Item{Amount: data.Amount, Price: data.Price})
	}

	orderbook.ProcessOrderbook(h.GetName(), currencyPair, orderBook, assetType)
	return orderbook.GetOrderbook(h.Name, currencyPair, assetType)
}

// GetAccountInfo retrieves balances for all enabled currencies for the
// HitBTC exchange
func (h *HitBTC) GetAccountInfo() (exchange.AccountInfo, error) {
	var response exchange.AccountInfo
	response.ExchangeName = h.GetName()
	accountBalance, err := h.GetBalances()
	if err != nil {
		return response, err
	}

	for _, item := range accountBalance {
		var exchangeCurrency exchange.AccountCurrencyInfo
		exchangeCurrency.CurrencyName = item.Currency
		exchangeCurrency.TotalValue = item.Available
		exchangeCurrency.Hold = item.Reserved
		response.Currencies = append(response.Currencies, exchangeCurrency)
	}
	return response, nil
}

// GetFundingHistory returns funding history, deposits and
// withdrawals
func (h *HitBTC) GetFundingHistory() ([]exchange.FundHistory, error) {
	var fundHistory []exchange.FundHistory
	return fundHistory, common.ErrFunctionNotSupported
}

// GetExchangeHistory returns historic trade data since exchange opening.
func (h *HitBTC) GetExchangeHistory(p pair.CurrencyPair, assetType assets.AssetType) ([]exchange.TradeHistory, error) {
	var resp []exchange.TradeHistory

	return resp, common.ErrNotYetImplemented
}

// SubmitOrder submits a new order
func (h *HitBTC) SubmitOrder(p pair.CurrencyPair, side exchange.OrderSide, orderType exchange.OrderType, amount, price float64, clientID string) (exchange.SubmitOrderResponse, error) {
	var submitOrderResponse exchange.SubmitOrderResponse
	response, err := h.PlaceOrder(p.Pair().String(), price, amount, common.StringToLower(orderType.ToString()), common.StringToLower(side.ToString()))

	if response.OrderNumber > 0 {
		submitOrderResponse.OrderID = fmt.Sprintf("%v", response.OrderNumber)
	}

	if err == nil {
		submitOrderResponse.IsOrderPlaced = true
	}

	return submitOrderResponse, err
}

// ModifyOrder will allow of changing orderbook placement and limit to
// market conversion
func (h *HitBTC) ModifyOrder(orderID int64, action exchange.ModifyOrder) (int64, error) {
	return 0, common.ErrNotYetImplemented
}

// CancelOrder cancels an order by its corresponding ID number
func (h *HitBTC) CancelOrder(order exchange.OrderCancellation) error {
	orderIDInt, err := strconv.ParseInt(order.OrderID, 10, 64)

	if err != nil {
		return err
	}

	_, err = h.CancelExistingOrder(orderIDInt)

	return err
}

// CancelAllOrders cancels all orders associated with a currency pair
func (h *HitBTC) CancelAllOrders() error {
	return common.ErrNotYetImplemented
}

// GetOrderInfo returns information on a current open order
func (h *HitBTC) GetOrderInfo(orderID int64) (exchange.OrderDetail, error) {
	var orderDetail exchange.OrderDetail
	return orderDetail, common.ErrNotYetImplemented
}

// GetDepositAddress returns a deposit address for a specified currency
func (h *HitBTC) GetDepositAddress(cryptocurrency pair.CurrencyItem) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawCryptocurrencyFunds returns a withdrawal ID when a withdrawal is
// submitted
func (h *HitBTC) WithdrawCryptocurrencyFunds(address string, cryptocurrency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawFiatFunds returns a withdrawal ID when a
// withdrawal is submitted
func (h *HitBTC) WithdrawFiatFunds(currency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawFiatFundsToInternationalBank returns a withdrawal ID when a
// withdrawal is submitted
func (h *HitBTC) WithdrawFiatFundsToInternationalBank(currency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// GetWebsocket returns a pointer to the exchange websocket
func (h *HitBTC) GetWebsocket() (*exchange.Websocket, error) {
	return h.Websocket, nil
}

// GetFeeByType returns an estimate of fee based on type of transaction
func (h *HitBTC) GetFeeByType(feeBuilder exchange.FeeBuilder) (float64, error) {
	return h.GetFee(feeBuilder)
}

// GetWithdrawCapabilities returns the types of withdrawal methods permitted by the exchange
func (h *HitBTC) GetWithdrawCapabilities() uint32 {
	return h.GetWithdrawPermissions()
}
