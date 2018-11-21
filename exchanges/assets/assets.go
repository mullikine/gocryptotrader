package assets

import "strings"

// AssetType stores the asset type
type AssetType string

// AssetTypes stores a list of asset types
type AssetTypes []AssetType

// Const vars for asset package
const (
	AssetTypeSpot = AssetType("Spot")

	AssetTypeMargin = AssetType("Margin")

	AssetTypeBinary = AssetType("Binary)")

	AssetTypeFutures        = AssetType("Futures")
	AssetTypeFutures1Week   = AssetType("Futures1Week")
	AssetTypeFutures2Weeks  = AssetType("Futures2Weeks")
	AssetTypeFutures1Month  = AssetType("Futures1Month")
	AssetTypeFutures2Months = AssetType("Futures2Months")
	AssetTypeFutures3Months = AssetType("Futures3Months")
	AssetTypeFutures6Months = AssetType("Futures6Months")
	AssetTypeFutures9Months = AssetType("Futures9Months")
	AssetTypeFutures1Year   = AssetType("Futures1Year")
)

// returns an AssetType to string
func (a AssetType) String() string {
	return string(a)
}

// ToStringArray converts an asset type array to a string array
func (a AssetTypes) ToStringArray() []string {
	var assets []string
	for x := range a {
		assets = append(assets, string(a[x]))
	}
	return assets
}

// JoinToString joins an asset type array and converts it to a string
// with the supplied seperator
func (a AssetTypes) JoinToString(separator string) string {
	return strings.Join(a.ToStringArray(), separator)
}

// New takes an input of asset types as string and returns an AssetTypes
// array
func New(input string) AssetTypes {
	assets := strings.Split(input, ",")
	var result AssetTypes
	for x := range assets {
		result = append(result, AssetType(assets[x]))
	}
	return result
}
