package okex

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

// SetDefaults method assignes the default values for Bittrex
func (o *OKEX) SetDefaults() {
	o.SetErrorDefaults()
	o.SetCheckVarDefaults()
	o.Name = "OKEX"
	o.Enabled = true
	o.Verbose = true
	o.APIWithdrawPermissions = exchange.AutoWithdrawCrypto
	o.RequestCurrencyPairFormat.Delimiter = "_"
	o.ConfigCurrencyPairFormat.Delimiter = "_"
	o.ConfigCurrencyPairFormat.Uppercase = true
	o.Features = exchange.Features{
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
	o.Requester = request.New(o.Name,
		request.NewRateLimit(time.Second, okexAuthRate),
		request.NewRateLimit(time.Second, okexUnauthRate),
		common.NewHTTPClientWithTimeout(exchange.DefaultHTTPTimeout))
	o.API.Endpoints.URLDefault = apiURL
	o.API.Endpoints.URL = o.API.Endpoints.URLDefault
	o.AssetTypes = assets.AssetTypes{assets.AssetTypeSpot}
	o.WebsocketInit()
}

// Setup method sets current configuration details if enabled
func (o *OKEX) Setup(exch config.ExchangeConfig) error {
	if !exch.Enabled {
		o.SetEnabled(false)
		return nil
	}

	err := o.SetupDefaults(exch)
	if err != nil {
		return err
	}

	return o.WebsocketSetup(o.WsConnect,
		exch.Name,
		exch.Features.Enabled.Websocket,
		okexDefaultWebsocketURL,
		exch.API.Endpoints.WebsocketURL)
}

// Start starts the OKEX go routine
func (o *OKEX) Start(wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		o.Run()
		wg.Done()
	}()
}

// Run implements the OKEX wrapper
func (o *OKEX) Run() {
	if o.Verbose {
		log.Printf("%s Websocket: %s. (url: %s).\n", o.GetName(), common.IsEnabled(o.Websocket.IsEnabled()), o.WebsocketURL)
		log.Printf("%s %d currencies enabled: %s.\n", o.GetName(), len(o.EnabledPairs), o.EnabledPairs)
	}

	if !o.GetEnabledFeatures().AutoPairUpdates {
		return
	}

	err := o.UpdateTradablePairs(false)
	if err != nil {
		log.Printf("%s failed to update tradable pairs. Err: %s", o.Name, err)
	}
}

// FetchTradablePairs returns a list of the exchanges tradable pairs
func (o *OKEX) FetchTradablePairs() ([]string, error) {
	prods, err := o.GetSpotInstruments()
	if err != nil {
		return nil, err
	}

	var pairs []string
	for x := range prods {
		pairs = append(pairs, prods[x].BaseCurrency+"_"+prods[x].QuoteCurrency)
	}

	return pairs, nil
}

// UpdateTradablePairs updates the exchanges available pairs and stores
// them in the exchanges config
func (o *OKEX) UpdateTradablePairs(forceUpdate bool) error {
	pairs, err := o.FetchTradablePairs()
	if err != nil {
		return err
	}

	return o.UpdatePairs(pairs, false, forceUpdate)
}

// UpdateTicker updates and returns the ticker for a currency pair
func (o *OKEX) UpdateTicker(p pair.CurrencyPair, assetType assets.AssetType) (ticker.Price, error) {
	currency := exchange.FormatExchangeCurrency(o.Name, p).String()
	var tickerPrice ticker.Price

	if assetType != assets.AssetTypeSpot {
		tick, err := o.GetContractPrice(currency, assetType.String())
		if err != nil {
			return tickerPrice, err
		}

		tickerPrice.Pair = p
		tickerPrice.Ask = tick.Ticker.Sell
		tickerPrice.Bid = tick.Ticker.Buy
		tickerPrice.Low = tick.Ticker.Low
		tickerPrice.Last = tick.Ticker.Last
		tickerPrice.Volume = tick.Ticker.Vol
		tickerPrice.High = tick.Ticker.High
		ticker.ProcessTicker(o.GetName(), p, tickerPrice, assetType)
	} else {
		tick, err := o.GetSpotTicker(currency)
		if err != nil {
			return tickerPrice, err
		}
		tickerPrice.Pair = p
		tickerPrice.Ask = tick.Ticker.Sell
		tickerPrice.Bid = tick.Ticker.Buy
		tickerPrice.Low = tick.Ticker.Low
		tickerPrice.Last = tick.Ticker.Last
		tickerPrice.Volume = tick.Ticker.Vol
		tickerPrice.High = tick.Ticker.High
		ticker.ProcessTicker(o.GetName(), p, tickerPrice, assets.AssetTypeSpot)

	}
	return ticker.GetTicker(o.Name, p, assetType)
}

// FetchTicker returns the ticker for a currency pair
func (o *OKEX) FetchTicker(p pair.CurrencyPair, assetType assets.AssetType) (ticker.Price, error) {
	tickerNew, err := ticker.GetTicker(o.GetName(), p, assetType)
	if err != nil {
		return o.UpdateTicker(p, assetType)
	}
	return tickerNew, nil
}

// FetchOrderbook returns orderbook base on the currency pair
func (o *OKEX) FetchOrderbook(currency pair.CurrencyPair, assetType assets.AssetType) (orderbook.Base, error) {
	ob, err := orderbook.GetOrderbook(o.GetName(), currency, assetType)
	if err != nil {
		return o.UpdateOrderbook(currency, assetType)
	}
	return ob, nil
}

// UpdateOrderbook updates and returns the orderbook for a currency pair
func (o *OKEX) UpdateOrderbook(p pair.CurrencyPair, assetType assets.AssetType) (orderbook.Base, error) {
	var orderBook orderbook.Base
	currency := exchange.FormatExchangeCurrency(o.Name, p).String()

	if assetType != assets.AssetTypeSpot {
		orderbookNew, err := o.GetContractMarketDepth(currency, assetType.String())
		if err != nil {
			return orderBook, err
		}

		for x := range orderbookNew.Bids {
			data := orderbookNew.Bids[x]
			orderBook.Bids = append(orderBook.Bids, orderbook.Item{Amount: data.Volume, Price: data.Price})
		}

		for x := range orderbookNew.Asks {
			data := orderbookNew.Asks[x]
			orderBook.Asks = append(orderBook.Asks, orderbook.Item{Amount: data.Volume, Price: data.Price})
		}

	} else {
		orderbookNew, err := o.GetSpotMarketDepth(ActualSpotDepthRequestParams{
			Symbol: currency,
			Size:   200,
		})
		if err != nil {
			return orderBook, err
		}

		for x := range orderbookNew.Bids {
			data := orderbookNew.Bids[x]
			orderBook.Bids = append(orderBook.Bids, orderbook.Item{Amount: data.Volume, Price: data.Price})
		}

		for x := range orderbookNew.Asks {
			data := orderbookNew.Asks[x]
			orderBook.Asks = append(orderBook.Asks, orderbook.Item{Amount: data.Volume, Price: data.Price})
		}
	}

	orderbook.ProcessOrderbook(o.GetName(), p, orderBook, assetType)
	return orderbook.GetOrderbook(o.Name, p, assetType)
}

// GetAccountInfo retrieves balances for all enabled currencies for the
// OKEX exchange
func (o *OKEX) GetAccountInfo() (exchange.AccountInfo, error) {
	var response exchange.AccountInfo
	return response, errors.New("not implemented")
}

// GetFundingHistory returns funding history, deposits and
// withdrawals
func (o *OKEX) GetFundingHistory() ([]exchange.FundHistory, error) {
	var fundHistory []exchange.FundHistory
	return fundHistory, common.ErrFunctionNotSupported
}

// GetExchangeHistory returns historic trade data since exchange opening.
func (o *OKEX) GetExchangeHistory(p pair.CurrencyPair, assetType assets.AssetType) ([]exchange.TradeHistory, error) {
	var resp []exchange.TradeHistory

	return resp, common.ErrNotYetImplemented
}

// SubmitOrder submits a new order
func (o *OKEX) SubmitOrder(p pair.CurrencyPair, side exchange.OrderSide, orderType exchange.OrderType, amount, price float64, clientID string) (exchange.SubmitOrderResponse, error) {
	var submitOrderResponse exchange.SubmitOrderResponse
	var oT SpotNewOrderRequestType

	if orderType == exchange.Limit {
		if side == exchange.Buy {
			oT = SpotNewOrderRequestTypeBuy
		} else {
			oT = SpotNewOrderRequestTypeSell
		}
	} else if orderType == exchange.Market {
		if side == exchange.Buy {
			oT = SpotNewOrderRequestTypeBuyMarket
		} else {
			oT = SpotNewOrderRequestTypeSellMarket
		}
	} else {
		return submitOrderResponse, errors.New("Unsupported order type")
	}

	var params = SpotNewOrderRequestParams{
		Amount: amount,
		Price:  price,
		Symbol: p.Pair().String(),
		Type:   oT,
	}

	response, err := o.SpotNewOrder(params)

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
func (o *OKEX) ModifyOrder(orderID int64, action exchange.ModifyOrder) (int64, error) {
	return 0, common.ErrNotYetImplemented
}

// CancelOrder cancels an order by its corresponding ID number
func (o *OKEX) CancelOrder(order exchange.OrderCancellation) error {
	orderIDInt, err := strconv.ParseInt(order.OrderID, 10, 64)

	if err != nil {
		return err
	}

	_, err = o.SpotCancelOrder(exchange.FormatExchangeCurrency(o.Name, order.CurrencyPair).String(), orderIDInt)

	return err
}

// CancelAllOrders cancels all orders associated with a currency pair
func (o *OKEX) CancelAllOrders() error {
	return common.ErrNotYetImplemented
}

// GetOrderInfo returns information on a current open order
func (o *OKEX) GetOrderInfo(orderID int64) (exchange.OrderDetail, error) {
	var orderDetail exchange.OrderDetail
	return orderDetail, common.ErrNotYetImplemented
}

// GetDepositAddress returns a deposit address for a specified currency
func (o *OKEX) GetDepositAddress(cryptocurrency pair.CurrencyItem) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawCryptocurrencyFunds returns a withdrawal ID when a withdrawal is
// submitted
func (o *OKEX) WithdrawCryptocurrencyFunds(address string, cryptocurrency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawFiatFunds returns a withdrawal ID when a
// withdrawal is submitted
func (o *OKEX) WithdrawFiatFunds(currency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawFiatFundsToInternationalBank returns a withdrawal ID when a
// withdrawal is submitted
func (o *OKEX) WithdrawFiatFundsToInternationalBank(currency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// GetWebsocket returns a pointer to the exchange websocket
func (o *OKEX) GetWebsocket() (*exchange.Websocket, error) {
	return o.Websocket, nil
}

// GetFeeByType returns an estimate of fee based on type of transaction
func (o *OKEX) GetFeeByType(feeBuilder exchange.FeeBuilder) (float64, error) {
	return o.GetFee(feeBuilder)
}

// GetWithdrawCapabilities returns the types of withdrawal methods permitted by the exchange
func (o *OKEX) GetWithdrawCapabilities() uint32 {
	return o.GetWithdrawPermissions()
}
