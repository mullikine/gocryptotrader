package kraken

import (
	"log"
	"strings"
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
func (k *Kraken) SetDefaults() {
	k.Name = "Kraken"
	k.Enabled = true
	k.Verbose = true
	k.APIWithdrawPermissions = exchange.AutoWithdrawCryptoWithSetup | exchange.WithdrawCryptoWith2FA | exchange.AutoWithdrawFiatWithSetup | exchange.WithdrawFiatWith2FA
	k.RequestCurrencyPairFormat.Uppercase = true
	k.RequestCurrencyPairFormat.Separator = ","
	k.ConfigCurrencyPairFormat.Delimiter = "-"
	k.ConfigCurrencyPairFormat.Uppercase = true
	k.AssetTypes = assets.AssetTypes{assets.AssetTypeSpot}
	k.Features = exchange.Features{
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
	k.Requester = request.New(k.Name,
		request.NewRateLimit(time.Second, krakenAuthRate),
		request.NewRateLimit(time.Second, krakenUnauthRate),
		common.NewHTTPClientWithTimeout(exchange.DefaultHTTPTimeout))
	k.API.Endpoints.URLDefault = krakenAPIURL
	k.API.Endpoints.URL = k.API.Endpoints.URLDefault
}

// Setup sets current exchange configuration
func (k *Kraken) Setup(exch config.ExchangeConfig) error {
	if !exch.Enabled {
		k.SetEnabled(false)
		return nil
	}

	return k.SetupDefaults(exch)
}

// Start starts the Kraken go routine
func (k *Kraken) Start(wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		k.Run()
		wg.Done()
	}()
}

// Run implements the Kraken wrapper
func (k *Kraken) Run() {
	if k.Verbose {
		log.Printf("%s %d currencies enabled: %s.\n", k.GetName(), len(k.EnabledPairs), k.EnabledPairs)
	}

	forceUpdate := false
	if !common.StringDataContains(k.EnabledPairs, "-") || !common.StringDataContains(k.AvailablePairs, "-") {
		enabledPairs := []string{"XBT-USD"}
		log.Println("WARNING: Available pairs for Kraken reset due to config upgrade, please enable the ones you would like again")
		forceUpdate = true

		err := k.UpdatePairs(enabledPairs, true, true)
		if err != nil {
			log.Printf("%s failed to update currencies. Err: %s\n", k.Name, err)
		}
	}

	if !k.GetEnabledFeatures().AutoPairUpdates && !forceUpdate {
		return
	}

	err := k.UpdateTradablePairs(forceUpdate)
	if err != nil {
		log.Printf("%s failed to update tradable pairs. Err: %s", k.Name, err)
	}
}

// FetchTradablePairs returns a list of the exchanges tradable pairs
func (k *Kraken) FetchTradablePairs() ([]string, error) {
	pairs, err := k.GetAssetPairs()
	if err != nil {
		return nil, err
	}

	var products []string
	for _, v := range pairs {
		if common.StringContains(v.Altname, ".d") {
			continue
		}
		if v.Base[0] == 'X' {
			if len(v.Base) > 3 {
				v.Base = v.Base[1:]
			}
		}
		if v.Quote[0] == 'Z' || v.Quote[0] == 'X' {
			v.Quote = v.Quote[1:]
		}
		products = append(products, v.Base+"-"+v.Quote)
	}

	return products, nil
}

// UpdateTradablePairs updates the exchanges available pairs and stores
// them in the exchanges config
func (k *Kraken) UpdateTradablePairs(forceUpdate bool) error {
	pairs, err := k.FetchTradablePairs()
	if err != nil {
		return err
	}

	return k.UpdatePairs(pairs, false, forceUpdate)
}

// UpdateTicker updates and returns the ticker for a currency pair
func (k *Kraken) UpdateTicker(p pair.CurrencyPair, assetType assets.AssetType) (ticker.Price, error) {
	var tickerPrice ticker.Price
	pairs := k.GetEnabledPairs()
	pairsCollated, err := exchange.FormatExchangeCurrencies(k.Name, pairs)
	if err != nil {
		return tickerPrice, err
	}
	tickers, err := k.GetTickers(pairsCollated.String())
	if err != nil {
		return tickerPrice, err
	}

	for _, x := range pairs {
		for y, z := range tickers {
			if common.StringContains(y, x.FirstCurrency.Upper().String()) && common.StringContains(y, x.SecondCurrency.Upper().String()) {
				var tp ticker.Price
				tp.Pair = x
				tp.Last = z.Last
				tp.Ask = z.Ask
				tp.Bid = z.Bid
				tp.High = z.High
				tp.Low = z.Low
				tp.Volume = z.Volume
				ticker.ProcessTicker(k.GetName(), x, tp, assetType)
			}
		}
	}
	return ticker.GetTicker(k.GetName(), p, assetType)
}

// FetchTicker returns the ticker for a currency pair
func (k *Kraken) FetchTicker(p pair.CurrencyPair, assetType assets.AssetType) (ticker.Price, error) {
	tickerNew, err := ticker.GetTicker(k.GetName(), p, assetType)
	if err != nil {
		return k.UpdateTicker(p, assetType)
	}
	return tickerNew, nil
}

// FetchOrderbook returns orderbook base on the currency pair
func (k *Kraken) FetchOrderbook(p pair.CurrencyPair, assetType assets.AssetType) (orderbook.Base, error) {
	ob, err := orderbook.GetOrderbook(k.GetName(), p, assetType)
	if err != nil {
		return k.UpdateOrderbook(p, assetType)
	}
	return ob, nil
}

// UpdateOrderbook updates and returns the orderbook for a currency pair
func (k *Kraken) UpdateOrderbook(p pair.CurrencyPair, assetType assets.AssetType) (orderbook.Base, error) {
	var orderBook orderbook.Base
	orderbookNew, err := k.GetDepth(exchange.FormatExchangeCurrency(k.GetName(), p).String())
	if err != nil {
		return orderBook, err
	}

	for x := range orderbookNew.Bids {
		orderBook.Bids = append(orderBook.Bids, orderbook.Item{Amount: orderbookNew.Bids[x].Amount, Price: orderbookNew.Bids[x].Price})
	}

	for x := range orderbookNew.Asks {
		orderBook.Asks = append(orderBook.Asks, orderbook.Item{Amount: orderbookNew.Asks[x].Amount, Price: orderbookNew.Asks[x].Price})
	}

	orderbook.ProcessOrderbook(k.GetName(), p, orderBook, assetType)
	return orderbook.GetOrderbook(k.Name, p, assetType)
}

// GetAccountInfo retrieves balances for all enabled currencies for the
// Kraken exchange - to-do
func (k *Kraken) GetAccountInfo() (exchange.AccountInfo, error) {
	var response exchange.AccountInfo
	response.ExchangeName = k.GetName()
	return response, nil
}

// GetFundingHistory returns funding history, deposits and
// withdrawals
func (k *Kraken) GetFundingHistory() ([]exchange.FundHistory, error) {
	var fundHistory []exchange.FundHistory
	return fundHistory, common.ErrFunctionNotSupported
}

// GetExchangeHistory returns historic trade data since exchange opening.
func (k *Kraken) GetExchangeHistory(p pair.CurrencyPair, assetType assets.AssetType) ([]exchange.TradeHistory, error) {
	var resp []exchange.TradeHistory

	return resp, common.ErrNotYetImplemented
}

// SubmitOrder submits a new order
func (k *Kraken) SubmitOrder(p pair.CurrencyPair, side exchange.OrderSide, orderType exchange.OrderType, amount, price float64, clientID string) (exchange.SubmitOrderResponse, error) {
	var submitOrderResponse exchange.SubmitOrderResponse
	var args = AddOrderOptions{}

	response, err := k.AddOrder(p.Pair().String(), side.ToString(), orderType.ToString(), amount, price, 0, 0, args)

	if len(response.TransactionIds) > 0 {
		submitOrderResponse.OrderID = strings.Join(response.TransactionIds, ", ")
	}

	if err == nil {
		submitOrderResponse.IsOrderPlaced = true
	}

	return submitOrderResponse, err
}

// ModifyOrder will allow of changing orderbook placement and limit to
// market conversion
func (k *Kraken) ModifyOrder(orderID int64, action exchange.ModifyOrder) (int64, error) {
	return 0, common.ErrNotYetImplemented
}

// CancelOrder cancels an order by its corresponding ID number
func (k *Kraken) CancelOrder(order exchange.OrderCancellation) error {
	_, err := k.CancelExistingOrder(order.OrderID)

	return err
}

// CancelAllOrders cancels all orders associated with a currency pair
func (k *Kraken) CancelAllOrders() error {
	return common.ErrNotYetImplemented
}

// GetOrderInfo returns information on a current open order
func (k *Kraken) GetOrderInfo(orderID int64) (exchange.OrderDetail, error) {
	var orderDetail exchange.OrderDetail
	return orderDetail, common.ErrNotYetImplemented
}

// GetDepositAddress returns a deposit address for a specified currency
func (k *Kraken) GetDepositAddress(cryptocurrency pair.CurrencyItem) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawCryptocurrencyFunds returns a withdrawal ID when a withdrawal is
// submitted
func (k *Kraken) WithdrawCryptocurrencyFunds(address string, cryptocurrency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawFiatFunds returns a withdrawal ID when a
// withdrawal is submitted
func (k *Kraken) WithdrawFiatFunds(currency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// WithdrawFiatFundsToInternationalBank returns a withdrawal ID when a
// withdrawal is submitted
func (k *Kraken) WithdrawFiatFundsToInternationalBank(currency pair.CurrencyItem, amount float64) (string, error) {
	return "", common.ErrNotYetImplemented
}

// GetWebsocket returns a pointer to the exchange websocket
func (k *Kraken) GetWebsocket() (*exchange.Websocket, error) {
	return nil, common.ErrNotYetImplemented
}

// GetFeeByType returns an estimate of fee based on type of transaction
func (k *Kraken) GetFeeByType(feeBuilder exchange.FeeBuilder) (float64, error) {
	return k.GetFee(feeBuilder)
}

// GetWithdrawCapabilities returns the types of withdrawal methods permitted by the exchange
func (k *Kraken) GetWithdrawCapabilities() uint32 {
	return k.GetWithdrawPermissions()
}
