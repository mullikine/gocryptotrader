package binance

import (
	"errors"
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

// SetDefaults sets the basic defaults for Binance
func (b *Binance) SetDefaults() {
	b.Name = "Binance"
	b.Enabled = true
	b.Verbose = true
	b.RequestCurrencyPairFormat.Uppercase = true
	b.ConfigCurrencyPairFormat.Delimiter = "-"
	b.ConfigCurrencyPairFormat.Uppercase = true
	b.AssetTypes = assets.AssetTypes{assets.AssetTypeSpot}
	b.Features = exchange.Features{
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
	b.APIWithdrawPermissions = exchange.AutoWithdrawCrypto
	b.SetValues()
	b.Requester = request.New(b.Name,
		request.NewRateLimit(time.Second, binanceAuthRate),
		request.NewRateLimit(time.Second, binanceUnauthRate),
		common.NewHTTPClientWithTimeout(exchange.DefaultHTTPTimeout))
	b.API.Endpoints.URLDefault = apiURL
	b.API.Endpoints.URL = b.API.Endpoints.URLDefault
	b.WebsocketInit()
}

// Setup takes in the supplied exchange configuration details and sets params
func (b *Binance) Setup(exch config.ExchangeConfig) error {
	if !exch.Enabled {
		b.SetEnabled(false)
		return nil
	}

	err := b.SetupDefaults(exch)
	if err != nil {
		return err
	}

	return b.WebsocketSetup(b.WSConnect,
		exch.Name,
		exch.Features.Enabled.Websocket,
		binanceDefaultWebsocketURL,
		exch.API.Endpoints.WebsocketURL)
}

// Start starts the Binance go routine
func (b *Binance) Start(wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		b.Run()
		wg.Done()
	}()
}

// Run implements the Binance wrapper
func (b *Binance) Run() {
	if b.Verbose {
		log.Printf("%s Websocket: %s. (url: %s).\n", b.GetName(), common.IsEnabled(b.Websocket.IsEnabled()), b.Websocket.GetWebsocketURL())
		log.Printf("%s %d currencies enabled: %s.\n", b.GetName(), len(b.EnabledPairs), b.EnabledPairs)
	}

	forceUpdate := false
	if !common.StringDataContains(b.EnabledPairs, "-") || !common.StringDataContains(b.AvailablePairs, "-") {
		enabledPairs := []string{"BTC-USDT"}
		log.Println("WARNING: Available pairs for Binance reset due to config upgrade, please enable the ones you would like again")
		forceUpdate = true

		err := b.UpdatePairs(enabledPairs, true, true)
		if err != nil {
			log.Printf("%s failed to update currencies. Err: %s\n", b.Name, err)
		}
	}

	if !b.GetEnabledFeatures().AutoPairUpdates && !forceUpdate {
		return
	}

	err := b.UpdateTradablePairs(forceUpdate)
	if err != nil {
		log.Printf("%s failed to update tradable pairs. Err: %s", b.Name, err)
	}
}

// FetchTradablePairs returns a list of the exchanges tradable pairs
func (b *Binance) FetchTradablePairs() ([]string, error) {
	var validCurrencyPairs []string

	info, err := b.GetExchangeInfo()
	if err != nil {
		return nil, err
	}

	for _, symbol := range info.Symbols {
		if symbol.Status == "TRADING" {
			validCurrencyPairs = append(validCurrencyPairs, symbol.BaseAsset+"-"+symbol.QuoteAsset)
		}
	}
	return validCurrencyPairs, nil
}

// UpdateTradablePairs updates the exchanges available pairs and stores
// them in the exchanges config
func (b *Binance) UpdateTradablePairs(forceUpdate bool) error {
	pairs, err := b.FetchTradablePairs()
	if err != nil {
		return err
	}

	return b.UpdatePairs(pairs, false, forceUpdate)
}

// UpdateTicker updates and returns the ticker for a currency pair
func (b *Binance) UpdateTicker(p pair.CurrencyPair, assetType assets.AssetType) (ticker.Price, error) {
	var tickerPrice ticker.Price

	tick, err := b.GetTickers()
	if err != nil {
		return tickerPrice, err
	}

	for _, x := range b.GetEnabledPairs() {
		curr := exchange.FormatExchangeCurrency(b.Name, x)
		for y := range tick {
			if tick[y].Symbol == curr.String() {
				tickerPrice.Pair = x
				tickerPrice.Ask = tick[y].AskPrice
				tickerPrice.Bid = tick[y].BidPrice
				tickerPrice.High = tick[y].HighPrice
				tickerPrice.Last = tick[y].LastPrice
				tickerPrice.Low = tick[y].LowPrice
				tickerPrice.Volume = tick[y].Volume
				ticker.ProcessTicker(b.Name, x, tickerPrice, assetType)
			}
		}
	}
	return ticker.GetTicker(b.Name, p, assetType)
}

// FetchTicker returns the ticker for a currency pair
func (b *Binance) FetchTicker(p pair.CurrencyPair, assetType assets.AssetType) (ticker.Price, error) {
	tickerNew, err := ticker.GetTicker(b.GetName(), p, assetType)
	if err != nil {
		return b.UpdateTicker(p, assetType)
	}
	return tickerNew, nil
}

// FetchOrderbook returns orderbook base on the currency pair
func (b *Binance) FetchOrderbook(currency pair.CurrencyPair, assetType assets.AssetType) (orderbook.Base, error) {
	ob, err := orderbook.GetOrderbook(b.GetName(), currency, assetType)
	if err != nil {
		return b.UpdateOrderbook(currency, assetType)
	}
	return ob, nil
}

// UpdateOrderbook updates and returns the orderbook for a currency pair
func (b *Binance) UpdateOrderbook(p pair.CurrencyPair, assetType assets.AssetType) (orderbook.Base, error) {
	var orderBook orderbook.Base
	orderbookNew, err := b.GetOrderBook(OrderBookDataRequestParams{Symbol: exchange.FormatExchangeCurrency(b.Name, p).String(), Limit: 1000})
	if err != nil {
		return orderBook, err
	}

	for _, bids := range orderbookNew.Bids {
		orderBook.Bids = append(orderBook.Bids, orderbook.Item{Amount: bids.Quantity, Price: bids.Price})
	}

	for _, asks := range orderbookNew.Asks {
		orderBook.Asks = append(orderBook.Asks, orderbook.Item{Amount: asks.Quantity, Price: asks.Price})
	}

	orderbook.ProcessOrderbook(b.GetName(), p, orderBook, assetType)
	return orderbook.GetOrderbook(b.Name, p, assetType)
}

// GetAccountInfo retrieves balances for all enabled currencies for the
// Bithumb exchange
func (b *Binance) GetAccountInfo() (exchange.AccountInfo, error) {
	var response exchange.AccountInfo
	return response, common.ErrNotYetImplemented
}

// GetFundingHistory returns funding history, deposits and
// withdrawals
func (b *Binance) GetFundingHistory() ([]exchange.FundHistory, error) {
	var fundHistory []exchange.FundHistory
	return fundHistory, common.ErrFunctionNotSupported
}

// GetExchangeHistory returns historic trade data since exchange opening.
func (b *Binance) GetExchangeHistory(p pair.CurrencyPair, assetType assets.AssetType) ([]exchange.TradeHistory, error) {
	var resp []exchange.TradeHistory

	return resp, common.ErrNotYetImplemented
}

// SubmitOrder submits a new order
func (b *Binance) SubmitOrder(p pair.CurrencyPair, side exchange.OrderSide, orderType exchange.OrderType, amount, price float64, clientID string) (exchange.SubmitOrderResponse, error) {
	var submitOrderResponse exchange.SubmitOrderResponse

	var sideType RequestParamsSideType
	if side == exchange.Buy {
		sideType = BinanceRequestParamsSideBuy
	} else {
		sideType = BinanceRequestParamsSideSell
	}

	var requestParamsOrderType RequestParamsOrderType
	if orderType == exchange.Market {
		requestParamsOrderType = BinanceRequestParamsOrderMarket
	} else if orderType == exchange.Limit {
		requestParamsOrderType = BinanceRequestParamsOrderLimit
	} else {
		submitOrderResponse.IsOrderPlaced = false
		return submitOrderResponse, errors.New("Unsupported order type")
	}

	var orderRequest = NewOrderRequest{
		Symbol:    p.FirstCurrency.String() + p.SecondCurrency.String(),
		Side:      sideType,
		Price:     price,
		Quantity:  amount,
		TradeType: requestParamsOrderType,
	}

	response, err := b.NewOrder(orderRequest)

	if response.OrderID > 0 {
		submitOrderResponse.OrderID = fmt.Sprintf("%v", response.OrderID)
	}

	if err == nil {
		submitOrderResponse.IsOrderPlaced = true
	}

	return submitOrderResponse, err
}

// ModifyOrder will allow of changing orderbook placement and limit to
// market conversion
func (b *Binance) ModifyOrder(orderID int64, action exchange.ModifyOrder) (int64, error) {
	return 0, common.ErrNotYetImplemented
}

// CancelOrder cancels an order by its corresponding ID number
func (b *Binance) CancelOrder(order exchange.OrderCancellation) error {
	orderIDInt, err := strconv.ParseInt(order.OrderID, 10, 64)

	if err != nil {
		return err
	}

	_, err = b.CancelExistingOrder(exchange.FormatExchangeCurrency(b.Name, order.CurrencyPair).String(), orderIDInt, order.AccountID)

	return err
}

// CancelAllOrders cancels all orders associated with a currency pair
func (b *Binance) CancelAllOrders() error {
	return common.ErrNotYetImplemented
}

// GetOrderInfo returns information on a current open order
func (b *Binance) GetOrderInfo(orderID int64) (exchange.OrderDetail, error) {
	var orderDetail exchange.OrderDetail
	return orderDetail, common.ErrNotYetImplemented
}

// GetDepositAddress returns a deposit address for a specified currency
func (b *Binance) GetDepositAddress(cryptocurrency pair.CurrencyItem) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawCryptocurrencyFunds returns a withdrawal ID when a withdrawal is
// submitted
func (b *Binance) WithdrawCryptocurrencyFunds(address string, cryptocurrency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawFiatFunds returns a withdrawal ID when a
// withdrawal is submitted
func (b *Binance) WithdrawFiatFunds(currency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawFiatFundsToInternationalBank returns a withdrawal ID when a
// withdrawal is submitted
func (b *Binance) WithdrawFiatFundsToInternationalBank(currency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// GetWebsocket returns a pointer to the exchange websocket
func (b *Binance) GetWebsocket() (*exchange.Websocket, error) {
	return b.Websocket, nil
}

// GetFeeByType returns an estimate of fee based on type of transaction
func (b *Binance) GetFeeByType(feeBuilder exchange.FeeBuilder) (float64, error) {
	return b.GetFee(feeBuilder)
}

// GetWithdrawCapabilities returns the types of withdrawal methods permitted by the exchange
func (b *Binance) GetWithdrawCapabilities() uint32 {
	return b.GetWithdrawPermissions()
}
