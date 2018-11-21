package coinbasepro

import (
	"errors"
	"log"
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

// SetDefaults sets default values for the exchange
func (c *CoinbasePro) SetDefaults() {
	c.Name = "CoinbasePro"
	c.Enabled = true
	c.Verbose = true
	c.APIWithdrawPermissions = exchange.AutoWithdrawCryptoWithAPIPermission | exchange.AutoWithdrawFiatWithAPIPermission
	c.RequestCurrencyPairFormat.Delimiter = "-"
	c.RequestCurrencyPairFormat.Uppercase = true
	c.ConfigCurrencyPairFormat.Uppercase = true
	c.AssetTypes = assets.AssetTypes{assets.AssetTypeSpot}
	c.Features = exchange.Features{
		Supports: exchange.FeaturesSupported{
			AutoPairUpdates:    true,
			RESTTickerBatching: false,
			REST:               true,
			Websocket:          true,
		},
		Enabled: exchange.FeaturesEnabled{
			AutoPairUpdates: true,
		},
	}
	c.Requester = request.New(c.Name,
		request.NewRateLimit(time.Second, coinbaseproAuthRate),
		request.NewRateLimit(time.Second, coinbaseproUnauthRate),
		common.NewHTTPClientWithTimeout(exchange.DefaultHTTPTimeout))
	c.API.Endpoints.URLDefault = coinbaseproAPIURL
	c.API.Endpoints.URL = c.API.Endpoints.URLDefault
	c.WebsocketInit()
	c.API.CredentialsValidator.RequiresClientID = true
	c.API.CredentialsValidator.RequiresBase64DecodeSecret = true
}

// Setup initialises the exchange parameters with the current configuration
func (c *CoinbasePro) Setup(exch config.ExchangeConfig) error {
	if !exch.Enabled {
		c.SetEnabled(false)
		return nil
	}

	err := c.SetupDefaults(exch)
	if err != nil {
		return err
	}

	return c.WebsocketSetup(c.WsConnect,
		exch.Name,
		exch.Features.Enabled.Websocket,
		coinbaseproWebsocketURL,
		exch.API.Endpoints.WebsocketURL)
}

// Start starts the coinbasepro go routine
func (c *CoinbasePro) Start(wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		c.Run()
		wg.Done()
	}()
}

// Run implements the coinbasepro wrapper
func (c *CoinbasePro) Run() {
	if c.Verbose {
		log.Printf("%s Websocket: %s. (url: %s).\n", c.GetName(), common.IsEnabled(c.Websocket.IsEnabled()), coinbaseproWebsocketURL)
		log.Printf("%s %d currencies enabled: %s.\n", c.GetName(), len(c.EnabledPairs), c.EnabledPairs)
	}

	if !c.GetEnabledFeatures().AutoPairUpdates {
		return
	}

	err := c.UpdateTradablePairs(false)
	if err != nil {
		log.Printf("%s failed to update tradable pairs. Err: %s", c.Name, err)
	}
}

// FetchTradablePairs returns a list of the exchanges tradable pairs
func (c *CoinbasePro) FetchTradablePairs() ([]string, error) {
	pairs, err := c.GetProducts()
	if err != nil {
		return nil, err
	}

	var products []string
	for x := range pairs {
		products = append(products, pairs[x].BaseCurrency+pairs[x].QuoteCurrency)
	}

	return products, nil
}

// UpdateTradablePairs updates the exchanges available pairs and stores
// them in the exchanges config
func (c *CoinbasePro) UpdateTradablePairs(forceUpdate bool) error {
	pairs, err := c.FetchTradablePairs()
	if err != nil {
		return err
	}

	return c.UpdatePairs(pairs, false, forceUpdate)
}

// GetAccountInfo retrieves balances for all enabled currencies for the
// coinbasepro exchange
func (c *CoinbasePro) GetAccountInfo() (exchange.AccountInfo, error) {
	var response exchange.AccountInfo
	response.ExchangeName = c.GetName()
	accountBalance, err := c.GetAccounts()
	if err != nil {
		return response, err
	}
	for i := 0; i < len(accountBalance); i++ {
		var exchangeCurrency exchange.AccountCurrencyInfo
		exchangeCurrency.CurrencyName = accountBalance[i].Currency
		exchangeCurrency.TotalValue = accountBalance[i].Available
		exchangeCurrency.Hold = accountBalance[i].Hold

		response.Currencies = append(response.Currencies, exchangeCurrency)
	}
	return response, nil
}

// UpdateTicker updates and returns the ticker for a currency pair
func (c *CoinbasePro) UpdateTicker(p pair.CurrencyPair, assetType assets.AssetType) (ticker.Price, error) {
	var tickerPrice ticker.Price
	tick, err := c.GetTicker(exchange.FormatExchangeCurrency(c.Name, p).String())
	if err != nil {
		return ticker.Price{}, err
	}

	stats, err := c.GetStats(exchange.FormatExchangeCurrency(c.Name, p).String())

	if err != nil {
		return ticker.Price{}, err
	}

	tickerPrice.Pair = p
	tickerPrice.Volume = stats.Volume
	tickerPrice.Last = tick.Price
	tickerPrice.High = stats.High
	tickerPrice.Low = stats.Low
	ticker.ProcessTicker(c.GetName(), p, tickerPrice, assetType)
	return ticker.GetTicker(c.Name, p, assetType)
}

// FetchTicker returns the ticker for a currency pair
func (c *CoinbasePro) FetchTicker(p pair.CurrencyPair, assetType assets.AssetType) (ticker.Price, error) {
	tickerNew, err := ticker.GetTicker(c.GetName(), p, assetType)
	if err != nil {
		return c.UpdateTicker(p, assetType)
	}
	return tickerNew, nil
}

// FetchOrderbook returns orderbook base on the currency pair
func (c *CoinbasePro) FetchOrderbook(p pair.CurrencyPair, assetType assets.AssetType) (orderbook.Base, error) {
	ob, err := orderbook.GetOrderbook(c.GetName(), p, assetType)
	if err != nil {
		return c.UpdateOrderbook(p, assetType)
	}
	return ob, nil
}

// UpdateOrderbook updates and returns the orderbook for a currency pair
func (c *CoinbasePro) UpdateOrderbook(p pair.CurrencyPair, assetType assets.AssetType) (orderbook.Base, error) {
	var orderBook orderbook.Base
	orderbookNew, err := c.GetOrderbook(exchange.FormatExchangeCurrency(c.Name, p).String(), 2)
	if err != nil {
		return orderBook, err
	}

	obNew := orderbookNew.(OrderbookL1L2)

	for x := range obNew.Bids {
		orderBook.Bids = append(orderBook.Bids, orderbook.Item{Amount: obNew.Bids[x].Amount, Price: obNew.Bids[x].Price})
	}

	for x := range obNew.Asks {
		orderBook.Asks = append(orderBook.Asks, orderbook.Item{Amount: obNew.Asks[x].Amount, Price: obNew.Asks[x].Price})
	}

	orderbook.ProcessOrderbook(c.GetName(), p, orderBook, assetType)
	return orderbook.GetOrderbook(c.Name, p, assetType)
}

// GetFundingHistory returns funding history, deposits and
// withdrawals
func (c *CoinbasePro) GetFundingHistory() ([]exchange.FundHistory, error) {
	var fundHistory []exchange.FundHistory
	return fundHistory, common.ErrFunctionNotSupported
}

// GetExchangeHistory returns historic trade data since exchange opening.
func (c *CoinbasePro) GetExchangeHistory(p pair.CurrencyPair, assetType assets.AssetType) ([]exchange.TradeHistory, error) {
	var resp []exchange.TradeHistory

	return resp, common.ErrNotYetImplemented
}

// SubmitOrder submits a new order
func (c *CoinbasePro) SubmitOrder(p pair.CurrencyPair, side exchange.OrderSide, orderType exchange.OrderType, amount, price float64, clientID string) (exchange.SubmitOrderResponse, error) {
	var submitOrderResponse exchange.SubmitOrderResponse
	var response string
	var err error
	if orderType == exchange.Market {
		response, err = c.PlaceMarginOrder("", amount, amount, side.ToString(), p.Pair().String(), "")

	} else if orderType == exchange.Limit {
		response, err = c.PlaceLimitOrder("", price, amount, side.ToString(), "", "", p.Pair().String(), "", false)
	} else {
		err = errors.New("not supported")
	}

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
func (c *CoinbasePro) ModifyOrder(orderID int64, action exchange.ModifyOrder) (int64, error) {
	return 0, common.ErrNotYetImplemented
}

// CancelOrder cancels an order by its corresponding ID number
func (c *CoinbasePro) CancelOrder(order exchange.OrderCancellation) error {
	return c.CancelExistingOrder(order.OrderID)
}

// CancelAllOrders cancels all orders associated with a currency pair
func (c *CoinbasePro) CancelAllOrders() error {
	return common.ErrNotYetImplemented
}

// GetOrderInfo returns information on a current open order
func (c *CoinbasePro) GetOrderInfo(orderID int64) (exchange.OrderDetail, error) {
	var orderDetail exchange.OrderDetail
	return orderDetail, common.ErrNotYetImplemented
}

// GetDepositAddress returns a deposit address for a specified currency
func (c *CoinbasePro) GetDepositAddress(cryptocurrency pair.CurrencyItem) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawCryptocurrencyFunds returns a withdrawal ID when a withdrawal is
// submitted
func (c *CoinbasePro) WithdrawCryptocurrencyFunds(address string, cryptocurrency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawFiatFunds returns a withdrawal ID when a withdrawal is
// submitted
func (c *CoinbasePro) WithdrawFiatFunds(cryptocurrency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// GetWebsocket returns a pointer to the exchange websocket
func (c *CoinbasePro) GetWebsocket() (*exchange.Websocket, error) {
	return c.Websocket, nil
}

// GetFeeByType returns an estimate of fee based on type of transaction
func (c *CoinbasePro) GetFeeByType(feeBuilder exchange.FeeBuilder) (float64, error) {
	return c.GetFee(feeBuilder)
}

// GetWithdrawCapabilities returns the types of withdrawal methods permitted by the exchange
func (c *CoinbasePro) GetWithdrawCapabilities() uint32 {
	return c.GetWithdrawPermissions()
}
