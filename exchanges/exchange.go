package exchange

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/thrasher-/gocryptotrader/common"
	"github.com/thrasher-/gocryptotrader/config"
	"github.com/thrasher-/gocryptotrader/currency/pair"
	"github.com/thrasher-/gocryptotrader/exchanges/assets"
	"github.com/thrasher-/gocryptotrader/exchanges/request"
)

const (
	warningBase64DecryptSecretKeyFailed = "WARNING -- Exchange %s unable to base64 decode secret key.. Disabling Authenticated API support."
	// WarningAuthenticatedRequestWithoutCredentialsSet error message for authenticated request without credentials set
	WarningAuthenticatedRequestWithoutCredentialsSet = "WARNING -- Exchange %s authenticated HTTP request called but not supported due to unset/default API keys."
	// ErrExchangeNotFound is a stand for an error message
	ErrExchangeNotFound = "Exchange not found in dataset"
	// DefaultHTTPTimeout is the default HTTP/HTTPS Timeout for exchange requests
	DefaultHTTPTimeout = time.Second * 15
)

// SetHTTPClientTimeout sets the timeout value for the exchanges
// HTTP Client
func (e *Base) SetHTTPClientTimeout(t time.Duration) {
	if e.Requester == nil {
		e.Requester = request.New(e.Name,
			request.NewRateLimit(time.Second, 0),
			request.NewRateLimit(time.Second, 0),
			new(http.Client))
	}
	e.Requester.HTTPClient.Timeout = t
}

// SetHTTPClient sets exchanges HTTP client
func (e *Base) SetHTTPClient(h *http.Client) {
	if e.Requester == nil {
		e.Requester = request.New(e.Name,
			request.NewRateLimit(time.Second, 0),
			request.NewRateLimit(time.Second, 0),
			new(http.Client))
	}
	e.Requester.HTTPClient = h
}

// GetHTTPClient gets the exchanges HTTP client
func (e *Base) GetHTTPClient() *http.Client {
	if e.Requester == nil {
		e.Requester = request.New(e.Name,
			request.NewRateLimit(time.Second, 0),
			request.NewRateLimit(time.Second, 0),
			new(http.Client))
	}
	return e.Requester.HTTPClient
}

// SetHTTPClientUserAgent sets the exchanges HTTP user agent
func (e *Base) SetHTTPClientUserAgent(ua string) {
	if e.Requester == nil {
		e.Requester = request.New(e.Name,
			request.NewRateLimit(time.Second, 0),
			request.NewRateLimit(time.Second, 0),
			new(http.Client))
	}
	e.Requester.UserAgent = ua
	e.HTTPUserAgent = ua
}

// GetHTTPClientUserAgent gets the exchanges HTTP user agent
func (e *Base) GetHTTPClientUserAgent() string {
	return e.HTTPUserAgent
}

// SetClientProxyAddress sets a proxy address for REST and websocket requests
func (e *Base) SetClientProxyAddress(addr string) error {
	if addr != "" {
		proxy, err := url.Parse(addr)
		if err != nil {
			return fmt.Errorf("exchange.go - setting proxy address error %s",
				err)
		}

		err = e.Requester.SetProxy(proxy)
		if err != nil {
			return fmt.Errorf("exchange.go - setting proxy address error %s",
				err)
		}

		if e.Websocket != nil {
			err = e.Websocket.SetProxyAddress(addr)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// SetFeatureDefaults sets the exchanges default feature
// support set
func (e *Base) SetFeatureDefaults() error {
	cfg := config.GetConfig()
	exch, err := cfg.GetExchangeConfig(e.Name)
	if err != nil {
		return err
	}
	var update bool

	if exch.Features == nil {
		update = true

		s := &config.FeaturesConfig{
			Supports: config.FeaturesSupportedConfig{
				Websocket:          e.Features.Supports.Websocket,
				REST:               e.Features.Supports.REST,
				RESTTickerBatching: e.Features.Supports.RESTTickerBatching,
			},
		}

		if exch.SupportsAutoPairUpdates != nil {
			s.Supports.AutoPairUpdates = *exch.SupportsAutoPairUpdates
			s.Enabled.AutoPairUpdates = *exch.SupportsAutoPairUpdates
		} else {
			s.Supports.AutoPairUpdates = e.Features.Supports.AutoPairUpdates
			s.Enabled.AutoPairUpdates = e.Features.Supports.AutoPairUpdates
			if !s.Supports.AutoPairUpdates {
				exch.PairsLastUpdated = time.Now().Unix()
				e.PairsLastUpdated = exch.PairsLastUpdated
			}
		}
		exch.Features = s
		exch.SupportsAutoPairUpdates = nil
	} else {
		if e.Features.Supports.AutoPairUpdates != exch.Features.Supports.AutoPairUpdates {
			exch.Features.Supports.AutoPairUpdates = e.Features.Supports.AutoPairUpdates

			if !exch.Features.Supports.AutoPairUpdates {
				exch.PairsLastUpdated = time.Now().Unix()
			}
			update = true
		}

		if e.Features.Supports.REST != exch.Features.Supports.REST {
			exch.Features.Supports.REST = e.Features.Supports.REST
			update = true
		}

		if e.Features.Supports.RESTTickerBatching != exch.Features.Supports.RESTTickerBatching {
			exch.Features.Supports.RESTTickerBatching = e.Features.Supports.RESTTickerBatching
			update = true
		}

		if e.Features.Supports.Websocket != exch.Features.Supports.Websocket {
			exch.Features.Supports.Websocket = e.Features.Supports.Websocket
			update = true
		}

		e.Features.Enabled.AutoPairUpdates = exch.Features.Enabled.AutoPairUpdates
	}
	if update {
		return cfg.UpdateExchangeConfig(exch)
	}
	return nil
}

// SetAPICredentialDefaults sets the API Credential validator defaults
func (e *Base) SetAPICredentialDefaults() error {
	cfg := config.GetConfig()
	exch, err := cfg.GetExchangeConfig(e.Name)
	if err != nil {
		return err
	}

	update := false

	// Exchange hardcoded settings take precedence and overwrite the config settings
	if exch.API.CredentialsValidator.RequiresBase64DecodeSecret != e.API.CredentialsValidator.RequiresBase64DecodeSecret {
		exch.API.CredentialsValidator.RequiresBase64DecodeSecret = e.API.CredentialsValidator.RequiresBase64DecodeSecret
		update = true
	}

	if exch.API.CredentialsValidator.RequiresClientID != e.API.CredentialsValidator.RequiresClientID {
		exch.API.CredentialsValidator.RequiresClientID = e.API.CredentialsValidator.RequiresClientID
		update = true
	}

	if exch.API.CredentialsValidator.RequiresPEM != e.API.CredentialsValidator.RequiresPEM {
		exch.API.CredentialsValidator.RequiresPEM = e.API.CredentialsValidator.RequiresPEM
		update = true
	}

	if update {
		return cfg.UpdateExchangeConfig(exch)
	}
	return nil
}

// SupportsRESTTickerBatchUpdates returns whether or not the
// exhange supports REST batch ticker fetching
func (e *Base) SupportsRESTTickerBatchUpdates() bool {
	return e.Features.Supports.RESTTickerBatching
}

// SupportsAutoPairUpdates returns whether or not the exchange supports
// auto currency pair updating
func (e *Base) SupportsAutoPairUpdates() bool {
	return e.Features.Supports.AutoPairUpdates
}

// GetLastPairsUpdateTime returns the unix timestamp of when the exchanges
// currency pairs were last updated
func (e *Base) GetLastPairsUpdateTime() int64 {
	return e.PairsLastUpdated
}

// SetAssetTypes checks the exchange asset types (whether it supports SPOT,
// Binary or Futures) and sets it to a default setting if it doesn't exist
func (e *Base) SetAssetTypes() error {
	cfg := config.GetConfig()
	exch, err := cfg.GetExchangeConfig(e.Name)
	if err != nil {
		return err
	}

	update := false
	if exch.AssetTypes == "" {
		exch.AssetTypes = e.AssetTypes.JoinToString(",")
		update = true
	} else {
		exch.AssetTypes = e.AssetTypes.JoinToString(",")
		update = true
	}

	if update {
		return cfg.UpdateExchangeConfig(exch)
	}

	return nil
}

// GetAssetTypes returns the available asset types for an individual exchange
func (e *Base) GetAssetTypes() assets.AssetTypes {
	return e.AssetTypes
}

// GetExchangeAssetTypes returns the asset types the exchange supports (Spot,
// binary, futures)
func GetExchangeAssetTypes(exchName string) (assets.AssetTypes, error) {
	cfg := config.GetConfig()
	exch, err := cfg.GetExchangeConfig(exchName)
	if err != nil {
		return nil, err
	}

	return assets.New(exch.AssetTypes), nil
}

// GetClientBankAccounts returns banking details associated with
// a client for withdrawal purposes
func (e *Base) GetClientBankAccounts(exchangeName, withdrawalCurrency string) (config.BankAccount, error) {
	cfg := config.GetConfig()
	return cfg.GetClientBankAccounts(exchangeName, withdrawalCurrency)
}

// GetExchangeBankAccounts returns banking details associated with an
// exchange for funding purposes
func (e *Base) GetExchangeBankAccounts(exchangeName, depositCurrency string) (config.BankAccount, error) {
	cfg := config.GetConfig()
	return cfg.GetExchangeBankAccounts(exchangeName, depositCurrency)
}

// CompareCurrencyPairFormats checks and returns whether or not the two supplied
// config currency pairs match
func CompareCurrencyPairFormats(pair1 config.CurrencyPairFormatConfig, pair2 *config.CurrencyPairFormatConfig) bool {
	if pair1.Delimiter != pair2.Delimiter ||
		pair1.Uppercase != pair2.Uppercase ||
		pair1.Separator != pair2.Separator ||
		pair1.Index != pair2.Index {
		return false
	}
	return true
}

// SetCurrencyPairFormat checks the exchange request and config currency pair
// formats and sets it to a default setting if it doesn't exist
func (e *Base) SetCurrencyPairFormat() error {
	cfg := config.GetConfig()
	exch, err := cfg.GetExchangeConfig(e.Name)
	if err != nil {
		return err
	}

	update := false
	if exch.RequestCurrencyPairFormat == nil {
		exch.RequestCurrencyPairFormat = &config.CurrencyPairFormatConfig{
			Delimiter: e.RequestCurrencyPairFormat.Delimiter,
			Uppercase: e.RequestCurrencyPairFormat.Uppercase,
			Separator: e.RequestCurrencyPairFormat.Separator,
			Index:     e.RequestCurrencyPairFormat.Index,
		}
		update = true
	} else {
		if CompareCurrencyPairFormats(e.RequestCurrencyPairFormat,
			exch.RequestCurrencyPairFormat) {
			e.RequestCurrencyPairFormat = *exch.RequestCurrencyPairFormat
		} else {
			*exch.RequestCurrencyPairFormat = e.RequestCurrencyPairFormat
			update = true
		}
	}

	if exch.ConfigCurrencyPairFormat == nil {
		exch.ConfigCurrencyPairFormat = &config.CurrencyPairFormatConfig{
			Delimiter: e.ConfigCurrencyPairFormat.Delimiter,
			Uppercase: e.ConfigCurrencyPairFormat.Uppercase,
			Separator: e.ConfigCurrencyPairFormat.Separator,
			Index:     e.ConfigCurrencyPairFormat.Index,
		}
		update = true
	} else {
		if CompareCurrencyPairFormats(e.ConfigCurrencyPairFormat,
			exch.ConfigCurrencyPairFormat) {
			e.ConfigCurrencyPairFormat = *exch.ConfigCurrencyPairFormat
		} else {
			*exch.ConfigCurrencyPairFormat = e.ConfigCurrencyPairFormat
			update = true
		}
	}

	if update {
		return cfg.UpdateExchangeConfig(exch)
	}
	return nil
}

// GetAuthenticatedAPISupport returns whether the exchange supports
// authenticated API requests
func (e *Base) GetAuthenticatedAPISupport() bool {
	return e.API.AuthenticatedSupport
}

// GetName is a method that returns the name of the exchange base
func (e *Base) GetName() string {
	return e.Name
}

// GetEnabledFeatures returns the exchanges enabled features
func (e *Base) GetEnabledFeatures() FeaturesEnabled {
	return e.Features.Enabled
}

// GetSupportedFeatures returns the exchanges supported features
func (e *Base) GetSupportedFeatures() FeaturesSupported {
	return e.Features.Supports
}

// GetEnabledPairs is a method that returns the enabled currency pairs of
// the exchange base
func (e *Base) GetEnabledPairs() []pair.CurrencyPair {
	return pair.FormatPairs(e.EnabledPairs,
		e.ConfigCurrencyPairFormat.Delimiter,
		e.ConfigCurrencyPairFormat.Index)
}

// GetAvailablePairs is a method that returns the available currency pairs
// of the exchange base
func (e *Base) GetAvailablePairs() []pair.CurrencyPair {
	return pair.FormatPairs(e.AvailablePairs,
		e.ConfigCurrencyPairFormat.Delimiter,
		e.ConfigCurrencyPairFormat.Index)
}

// SupportsPair returns true or not whether a currency pair exists in the
// exchange available currencies or not
func (e *Base) SupportsPair(p pair.CurrencyPair, enabledPairs bool) bool {
	if enabledPairs {
		return pair.Contains(e.GetEnabledPairs(), p, false)
	}
	return pair.Contains(e.GetAvailablePairs(), p, false)
}

// GetExchangeFormatCurrencySeperator returns whether or not a specific
// exchange contains a separator used for API requests
func GetExchangeFormatCurrencySeperator(exchName string) bool {
	cfg := config.GetConfig()
	exch, err := cfg.GetExchangeConfig(exchName)
	if err != nil {
		return false
	}

	if exch.RequestCurrencyPairFormat.Separator != "" {
		return true
	}
	return false
}

// FormatExchangeCurrencies returns a pair.CurrencyItem string containing
// the exchanges formatted currency pairs
func FormatExchangeCurrencies(exchName string, pairs []pair.CurrencyPair) (pair.CurrencyItem, error) {
	var currencyItems pair.CurrencyItem
	cfg := config.GetConfig()
	exch, err := cfg.GetExchangeConfig(exchName)
	if err != nil {
		return currencyItems, err
	}

	for x := range pairs {
		currencyItems += FormatExchangeCurrency(exchName, pairs[x])
		if x == len(pairs)-1 {
			continue
		}
		currencyItems += pair.CurrencyItem(exch.RequestCurrencyPairFormat.Separator)
	}
	return currencyItems, nil
}

// FormatExchangeCurrency is a method that formats and returns a currency pair
// based on the user currency display preferences
func FormatExchangeCurrency(exchName string, p pair.CurrencyPair) pair.CurrencyItem {
	cfg := config.GetConfig()
	exch, _ := cfg.GetExchangeConfig(exchName)

	return p.Display(exch.RequestCurrencyPairFormat.Delimiter,
		exch.RequestCurrencyPairFormat.Uppercase)
}

// FormatCurrency is a method that formats and returns a currency pair
// based on the user currency display preferences
func FormatCurrency(p pair.CurrencyPair) pair.CurrencyItem {
	cfg := config.GetConfig()
	return p.Display(cfg.Currency.CurrencyPairFormat.Delimiter,
		cfg.Currency.CurrencyPairFormat.Uppercase)
}

// SetEnabled is a method that sets if the exchange is enabled
func (e *Base) SetEnabled(enabled bool) {
	e.Enabled = enabled
}

// IsEnabled is a method that returns if the current exchange is enabled
func (e *Base) IsEnabled() bool {
	return e.Enabled
}

// SetAPIKeys is a method that sets the current API keys for the exchange
func (e *Base) SetAPIKeys(APIKey, APISecret, ClientID string, b64Decode bool) {
	if !e.API.AuthenticatedSupport {
		return
	}

	e.API.Credentials.Key = APIKey
	e.API.Credentials.ClientID = ClientID

	if b64Decode {
		result, err := common.Base64Decode(APISecret)
		if err != nil {
			e.API.AuthenticatedSupport = false
			log.Printf(warningBase64DecryptSecretKeyFailed, e.Name)
		}
		e.API.Credentials.Secret = string(result)
	} else {
		e.API.Credentials.Secret = APISecret
	}
}

// SetupDefaults sets the exchange settings based on the supplied config
func (e *Base) SetupDefaults(exch config.ExchangeConfig) error {
	e.Enabled = true
	e.API.AuthenticatedSupport = exch.API.AuthenticatedSupport
	e.SetAPIKeys(exch.API.Credentials.Key, exch.API.Credentials.Secret, e.API.Credentials.ClientID, e.API.CredentialsValidator.RequiresBase64DecodeSecret)
	e.SetHTTPClientTimeout(exch.HTTPTimeout)
	e.SetHTTPClientUserAgent(exch.HTTPUserAgent)
	e.Verbose = exch.Verbose
	e.BaseCurrencies = common.SplitStrings(exch.BaseCurrencies, ",")
	e.AvailablePairs = common.SplitStrings(exch.AvailablePairs, ",")
	e.EnabledPairs = common.SplitStrings(exch.EnabledPairs, ",")

	err := e.SetCurrencyPairFormat()
	if err != nil {
		return err
	}
	err = e.SetAssetTypes()
	if err != nil {
		return err
	}
	err = e.SetFeatureDefaults()
	if err != nil {
		return err
	}
	err = e.SetAPIURL(exch)
	if err != nil {
		return err
	}
	err = e.SetClientProxyAddress(exch.ProxyAddress)
	if err != nil {
		return err
	}
	err = e.SetAPICredentialDefaults()
	if err != nil {
		return err
	}

	if e.Features.Supports.Websocket {
		e.Websocket.SetEnabled(exch.Features.Enabled.Websocket)
	}

	return nil
}

// ValidateAPICredentials validates the exchanges API credentials
func (e *Base) ValidateAPICredentials() bool {
	if e.API.Credentials.Key == "" || e.API.Credentials.Key == config.DefaultAPIKey {
		return false
	}

	if e.API.Credentials.Secret == "" || e.API.Credentials.Secret == config.DefaultAPISecret {
		return false
	}

	if e.API.CredentialsValidator.RequiresPEM {
		if e.API.Credentials.PEMKey == "" || common.StringContains(e.API.Credentials.PEMKey, "JUSTADUMMY") {
			return false
		}
	}

	if e.API.CredentialsValidator.RequiresClientID {
		if e.API.Credentials.ClientID == "" || e.API.Credentials.ClientID == config.DefaultAPIClientID {
			return false
		}
	}

	if e.API.CredentialsValidator.RequiresBase64DecodeSecret {
		_, err := common.Base64Decode(e.API.Credentials.Secret)
		if err != nil {
			return false
		}
	}

	return true
}

// SetPairs sets the exchange currency pairs for either enabledPairs or
// availablePairs
func (e *Base) SetPairs(pairs []pair.CurrencyPair, enabledPairs bool) error {
	if len(pairs) == 0 {
		return fmt.Errorf("%s SetPairs error - pairs is empty", e.Name)
	}

	cfg := config.GetConfig()
	exchCfg, err := cfg.GetExchangeConfig(e.Name)
	if err != nil {
		return err
	}

	var pairsStr []string
	for x := range pairs {
		pairsStr = append(pairsStr, pairs[x].Display(exchCfg.ConfigCurrencyPairFormat.Delimiter,
			exchCfg.ConfigCurrencyPairFormat.Uppercase).String())
	}

	if enabledPairs {
		exchCfg.EnabledPairs = common.JoinStrings(pairsStr, ",")
		e.EnabledPairs = pairsStr
	} else {
		exchCfg.AvailablePairs = common.JoinStrings(pairsStr, ",")
		e.AvailablePairs = pairsStr
	}

	return cfg.UpdateExchangeConfig(exchCfg)
}

// UpdatePairs updates the exchange currency pairs for either enabledPairs or
// availablePairs
func (e *Base) UpdatePairs(exchangeProducts []string, enabled, force bool) error {
	if len(exchangeProducts) == 0 {
		return fmt.Errorf("%s UpdatePairs error - exchangeProducts is empty", e.Name)
	}

	exchangeProducts = common.SplitStrings(common.StringToUpper(common.JoinStrings(exchangeProducts, ",")), ",")
	var products []string

	for x := range exchangeProducts {
		if exchangeProducts[x] == "" {
			continue
		}
		products = append(products, exchangeProducts[x])
	}

	var newPairs, removedPairs []string
	var updateType string

	if enabled {
		newPairs, removedPairs = pair.FindPairDifferences(e.EnabledPairs, products)
		updateType = "enabled"
	} else {
		newPairs, removedPairs = pair.FindPairDifferences(e.AvailablePairs, products)
		updateType = "available"
	}

	if force || len(newPairs) > 0 || len(removedPairs) > 0 {
		cfg := config.GetConfig()
		exch, err := cfg.GetExchangeConfig(e.Name)
		if err != nil {
			return err
		}

		if force {
			log.Printf("%s forced update of %s pairs.", e.Name, updateType)
		} else {
			if len(newPairs) > 0 {
				log.Printf("%s Updating pairs - New: %s.\n", e.Name, newPairs)
			}
			if len(removedPairs) > 0 {
				log.Printf("%s Updating pairs - Removed: %s.\n", e.Name, removedPairs)
			}
		}

		if enabled {
			exch.EnabledPairs = common.JoinStrings(products, ",")
			e.EnabledPairs = products
		} else {
			exch.AvailablePairs = common.JoinStrings(products, ",")
			e.AvailablePairs = products
		}
		return cfg.UpdateExchangeConfig(exch)
	}
	return nil
}

// SetAPIURL sets configuration API URL for an exchange
func (e *Base) SetAPIURL(ec config.ExchangeConfig) error {
	if ec.API.Endpoints.URL == "" || ec.API.Endpoints.URLSecondary == "" {
		return fmt.Errorf("exchange %s: SetAPIURL error. URL vals are empty", e.Name)
	}
	if ec.API.Endpoints.URL != config.APIURLNonDefaultMessage {
		e.API.Endpoints.URL = ec.API.Endpoints.URL
	}
	if ec.API.Endpoints.URLSecondary != config.APIURLNonDefaultMessage {
		e.API.Endpoints.URLSecondary = ec.API.Endpoints.URLSecondary
	}
	return nil
}

// GetAPIURL returns the set API URL
func (e *Base) GetAPIURL() string {
	return e.API.Endpoints.URL
}

// GetSecondaryAPIURL returns the set Secondary API URL
func (e *Base) GetSecondaryAPIURL() string {
	return e.API.Endpoints.URLSecondary
}

// GetAPIURLDefault returns exchange default URL
func (e *Base) GetAPIURLDefault() string {
	return e.API.Endpoints.URLDefault
}

// GetAPIURLSecondaryDefault returns exchange default secondary URL
func (e *Base) GetAPIURLSecondaryDefault() string {
	return e.API.Endpoints.URLSecondaryDefault
}

// SupportsWebsocket returns whether or not the exchange supports
// websocket
func (e *Base) SupportsWebsocket() bool {
	return e.Features.Supports.Websocket
}

// SupportsREST returns whether or not the exchange supports
// REST
func (e *Base) SupportsREST() bool {
	return e.Features.Supports.REST
}

// IsWebsocketEnabled returns whether or not the exchange has its
// websocket client enabled
func (e *Base) IsWebsocketEnabled() bool {
	if e.Websocket != nil {
		return e.Websocket.IsEnabled()
	}
	return false
}

// GetWithdrawPermissions passes through the exchange's withdraw permissions
func (e *Base) GetWithdrawPermissions() uint32 {
	return e.APIWithdrawPermissions
}

// SupportsWithdrawPermissions compares the supplied permissions with the exchange's to verify they're supported
func (e *Base) SupportsWithdrawPermissions(permissions uint32) bool {
	exchangePermissions := e.GetWithdrawPermissions()
	if permissions&exchangePermissions == permissions {
		return true
	}
	return false
}

// FormatWithdrawPermissions will return each of the exchange's compatible withdrawal methods in readable form
func (e *Base) FormatWithdrawPermissions() string {
	services := []string{}
	for i := 0; i < 32; i++ {
		var check uint32 = 1 << uint32(i)
		if e.GetWithdrawPermissions()&check != 0 {
			switch check {
			case AutoWithdrawCrypto:
				services = append(services, AutoWithdrawCryptoText)
			case AutoWithdrawCryptoWithAPIPermission:
				services = append(services, AutoWithdrawCryptoWithAPIPermissionText)
			case AutoWithdrawCryptoWithSetup:
				services = append(services, AutoWithdrawCryptoWithSetupText)
			case WithdrawCryptoWith2FA:
				services = append(services, WithdrawCryptoWith2FAText)
			case WithdrawCryptoWithSMS:
				services = append(services, WithdrawCryptoWithSMSText)
			case WithdrawCryptoWithEmail:
				services = append(services, WithdrawCryptoWithEmailText)
			case WithdrawCryptoWithWebsiteApproval:
				services = append(services, WithdrawCryptoWithWebsiteApprovalText)
			case WithdrawCryptoWithAPIPermission:
				services = append(services, WithdrawCryptoWithAPIPermissionText)
			case AutoWithdrawFiat:
				services = append(services, AutoWithdrawFiatText)
			case AutoWithdrawFiatWithAPIPermission:
				services = append(services, AutoWithdrawFiatWithAPIPermissionText)
			case AutoWithdrawFiatWithSetup:
				services = append(services, AutoWithdrawFiatWithSetupText)
			case WithdrawFiatWith2FA:
				services = append(services, WithdrawFiatWith2FAText)
			case WithdrawFiatWithSMS:
				services = append(services, WithdrawFiatWithSMSText)
			case WithdrawFiatWithEmail:
				services = append(services, WithdrawFiatWithEmailText)
			case WithdrawFiatWithWebsiteApproval:
				services = append(services, WithdrawFiatWithWebsiteApprovalText)
			case WithdrawFiatWithAPIPermission:
				services = append(services, WithdrawFiatWithAPIPermissionText)
			case WithdrawCryptoViaWebsiteOnly:
				services = append(services, WithdrawCryptoViaWebsiteOnlyText)
			case WithdrawFiatViaWebsiteOnly:
				services = append(services, WithdrawFiatViaWebsiteOnlyText)
			default:
				services = append(services, fmt.Sprintf("%s[1<<%v]", UnknownWithdrawalTypeText, i))
			}
		}
	}
	if len(services) > 0 {
		return strings.Join(services, " & ")
	}

	return NoAPIWithdrawalMethodsText
}
