package trader

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"nofx/logger"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

const (
	// Nado API endpoints
	NadoMainnetGateway  = "https://gateway.prod.nado.xyz/v1"
	NadoTestnetGateway  = "https://gateway.test.nado.xyz/v1"
	NadoMainnetArchive  = "https://archive.prod.nado.xyz/v1"
	NadoTestnetArchive  = "https://archive.test.nado.xyz/v1"
)

// NadoTrader Nado DEX trader
type NadoTrader struct {
	gatewayURL string    // Gateway API URL
	archiveURL string    // Archive API URL
	privateKey *ecdsa.PrivateKey
	walletAddr common.Address
	testnet    bool
	httpClient *httpClient
	nonceMutex sync.Mutex
	lastNonce  uint64
	meta       *NadoMeta     // Market metadata
	metaMutex  sync.RWMutex  // Protect meta access
}

// NadoMeta Nado market metadata
type NadoMeta struct {
	Assets  []NadoAsset  `json:"assets"`
	Pairs   []NadoPair   `json:"pairs"`
	sync    sync.RWMutex
}

// NadoAsset Asset information
type NadoAsset struct {
	Symbol      string `json:"symbol"`
	Name        string `json:"name"`
	Decimals    int    `json:"decimals"`
	AssetID     int    `json:"asset_id"`
}

// NadoPair Trading pair information
type NadoPair struct {
	TickerID     string  `json:"ticker_id"`
	BaseAssetID  int     `json:"base_asset_id"`
	QuoteAssetID int     `json:"quote_asset_id"`
	MinSize      string  `json:"min_size"`
	MaxSize      string  `json:"max_size"`
	TickSize     string  `json:"tick_size"`
	MakeFee      string  `json:"make_fee"`
	TakeFee      string  `json:"take_fee"`
}

// NadoExecuteResponse Execute API response
type NadoExecuteResponse struct {
	Response json.RawMessage `json:"response"`
	Error    *NadoError      `json:"error,omitempty"`
}

// NadoError Nado API error
type NadoError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// NadoSubaccountInfo Subaccount information
type NadoSubaccountInfo struct {
	MarginAccount  NadoMarginAccount `json:"margin_account"`
	PerpAccount    NadoPerpAccount   `json:"perp_account"`
	Health         string            `json:"health"`
	Products       []NadoProduct     `json:"products"`
}

// NadoMarginAccount Margin account info
type NadoMarginAccount struct {
	CollateralValue string            `json:"collateral_value"`
	TotalValue      string            `json:"total_value"`
	Balances        []NadoBalance     `json:"balances"`
}

// NadoPerpAccount Perpetual account info
type NadoPerpAccount struct {
	TotalMargin    string            `json:"total_margin"`
	TotalValue     string            `json:"total_value"`
	UnrealizedPnL  string            `json:"unrealized_pnl"`
	Balances       []NadoBalance     `json:"balances"`
	Positions      []NadoPosition    `json:"positions"`
}

// NadoBalance Balance information
type NadoBalance struct {
	AssetID   int    `json:"asset_id"`
	Symbol    string `json:"symbol"`
	Amount    string `json:"amount"`
}

// NadoPosition Position information
type NadoPosition struct {
	ProductID   int     `json:"product_id"`
	TickerID    string  `json:"ticker_id"`
	Side        string  `json:"side"` // "long" or "short"
	Size        string  `json:"size"`
	EntryPrice  string  `json:"entry_price"`
	MarkPrice   string  `json:"mark_price"`
	LiquidationPrice string `json:"liquidation_price"`
	UnrealizedPnL string `json:"unrealized_pnl"`
	Leverage    int     `json:"leverage"`
}

// NadoProduct Product information
type NadoProduct struct {
	ProductID   int     `json:"product_id"`
	TickerID    string  `json:"ticker_id"`
	MarkPrice   string  `json:"mark_price"`
	IndexPrice  string  `json:"index_price"`
	FundingRate string  `json:"funding_rate"`
}

// NadoOrderRequest Order request
type NadoOrderRequest struct {
	Sender     string `json:"sender"`
	PriceX18   string `json:"priceX18"`
	Amount     string `json:"amount"`
	Expiration string `json:"expiration"`
	Nonce      string `json:"nonce"`
	Appendix   string `json:"appendix"`
}

// httpClient Simple HTTP client wrapper
type httpClient struct {
	baseURL    string
	timeout    time.Duration
}

// NewNadoTrader creates a new Nado trader
func NewNadoTrader(privateKeyHex string, testnet bool) (*NadoTrader, error) {
	// Parse private key
	privateKeyHex = strings.TrimPrefix(privateKeyHex, "0x")
	if len(privateKeyHex) != 64 {
		return nil, fmt.Errorf("invalid private key length: expected 64 characters, got %d", len(privateKeyHex))
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Derive wallet address
	walletAddr := crypto.PubkeyToAddress(privateKey.PublicKey)

	// Select API endpoints
	gatewayURL := NadoMainnetGateway
	archiveURL := NadoMainnetArchive
	if testnet {
		gatewayURL = NadoTestnetGateway
		archiveURL = NadoTestnetArchive
	}

	trader := &NadoTrader{
		gatewayURL: gatewayURL,
		archiveURL: archiveURL,
		privateKey: privateKey,
		walletAddr: walletAddr,
		testnet:    testnet,
		httpClient: &httpClient{
			baseURL: gatewayURL,
			timeout: 30 * time.Second,
		},
	}

	logger.Infof("🔓 Nado trader initialized (testnet=%v, wallet=%s)", testnet, walletAddr.Hex())

	// Fetch market metadata
	if err := trader.refreshMeta(); err != nil {
		logger.Infof("⚠️  Failed to fetch market metadata: %v", err)
		// Don't fail initialization, metadata can be refreshed later
	}

	return trader, nil
}

// refreshMeta fetches and caches market metadata
func (t *NadoTrader) refreshMeta() error {
	// Fetch assets
	assetsData, err := t.httpClient.get("/assets")
	if err != nil {
		return fmt.Errorf("failed to fetch assets: %w", err)
	}

	var assets []NadoAsset
	if err := json.Unmarshal(assetsData, &assets); err != nil {
		return fmt.Errorf("failed to parse assets: %w", err)
	}

	// Fetch pairs (perpetuals)
	pairsData, err := t.httpClient.get("/pairs?market=perp")
	if err != nil {
		return fmt.Errorf("failed to fetch pairs: %w", err)
	}

	var pairs []NadoPair
	if err := json.Unmarshal(pairsData, &pairs); err != nil {
		return fmt.Errorf("failed to parse pairs: %w", err)
	}

	t.metaMutex.Lock()
	t.meta = &NadoMeta{
		Assets: assets,
		Pairs:  pairs,
	}
	t.metaMutex.Unlock()

	logger.Infof("✓ Nado metadata refreshed: %d assets, %d perp pairs", len(assets), len(pairs))
	return nil
}

// GetBalance gets account balance
func (t *NadoTrader) GetBalance() (map[string]interface{}, error) {
	subaccount := t.walletAddr.Hex()

	// Query subaccount info
	data, err := t.query("subaccount_info", map[string]string{
		"subaccount": subaccount,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get account info: %w", err)
	}

	var info NadoSubaccountInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("failed to parse account info: %w", err)
	}

	// Parse values
	totalEquity, _ := strconv.ParseFloat(info.PerpAccount.TotalValue, 64)
	unrealizedPnl, _ := strconv.ParseFloat(info.PerpAccount.UnrealizedPnL, 64)
	totalMargin, _ := strconv.ParseFloat(info.PerpAccount.TotalMargin, 64)

	// Calculate available balance (total equity - used margin)
	availableBalance := totalEquity - totalMargin
	if availableBalance < 0 {
		availableBalance = 0
	}

	walletBalance := totalEquity - unrealizedPnl

	result := map[string]interface{}{
		"totalWalletBalance":    walletBalance,
		"availableBalance":      availableBalance,
		"totalUnrealizedProfit":  unrealizedPnl,
		"totalEquity":           totalEquity,
		"marginUsed":            totalMargin,
	}

	logger.Infof("📊 Nado balance: equity=%.2f, available=%.2f, unrealized=%.2f",
		totalEquity, availableBalance, unrealizedPnl)

	return result, nil
}

// GetPositions gets all positions
func (t *NadoTrader) GetPositions() ([]map[string]interface{}, error) {
	subaccount := t.walletAddr.Hex()

	// Query subaccount info
	data, err := t.query("subaccount_info", map[string]string{
		"subaccount": subaccount,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get account info: %w", err)
	}

	var info NadoSubaccountInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("failed to parse account info: %w", err)
	}

	var result []map[string]interface{}

	// Process perpetual positions
	for _, pos := range info.PerpAccount.Positions {
		size, _ := strconv.ParseFloat(pos.Size, 64)
		if size == 0 {
			continue // Skip empty positions
		}

		entryPrice, _ := strconv.ParseFloat(pos.EntryPrice, 64)
		markPrice, _ := strconv.ParseFloat(pos.MarkPrice, 64)
		liqPrice, _ := strconv.ParseFloat(pos.LiquidationPrice, 64)
		unrealizedPnl, _ := strconv.ParseFloat(pos.UnrealizedPnL, 64)
		leverage := pos.Leverage

		// Normalize symbol (Nado uses BTC-PERP_USDT0, convert to BTCUSDT)
		symbol := normalizeNadoSymbol(pos.TickerID)

		posMap := map[string]interface{}{
			"symbol":             symbol,
			"side":               pos.Side,
			"positionAmt":        size,
			"entryPrice":         entryPrice,
			"markPrice":          markPrice,
			"unRealizedProfit":   unrealizedPnl,
			"leverage":           float64(leverage),
			"liquidationPrice":   liqPrice,
		}

		result = append(result, posMap)
	}

	return result, nil
}

// OpenLong opens a long position
func (t *NadoTrader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	logger.Infof("📈 Opening Nado long position: %s qty=%.4f lev=%dx", symbol, quantity, leverage)

	// Convert symbol to Nado format
	tickerID := convertToNadoSymbol(symbol)

	// Get current price
	price, err := t.GetMarketPrice(symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get market price: %w", err)
	}

	// Cancel existing orders first
	if err := t.CancelAllOrders(symbol); err != nil {
		logger.Infof("⚠️  Failed to cancel existing orders: %v", err)
	}

	// Place buy order (IOC for immediate fill)
	orderID, err := t.placeOrder(tickerID, price, quantity, true, leverage, false, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to place buy order: %w", err)
	}

	logger.Infof("✓ Nado long position opened: %s qty=%.4f orderID=%s", symbol, quantity, orderID)

	return map[string]interface{}{
		"orderId": orderID,
		"symbol":  symbol,
		"status":  "FILLED",
	}, nil
}

// OpenShort opens a short position
func (t *NadoTrader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	logger.Infof("📉 Opening Nado short position: %s qty=%.4f lev=%dx", symbol, quantity, leverage)

	// Convert symbol to Nado format
	tickerID := convertToNadoSymbol(symbol)

	// Get current price
	price, err := t.GetMarketPrice(symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get market price: %w", err)
	}

	// Cancel existing orders first
	if err := t.CancelAllOrders(symbol); err != nil {
		logger.Infof("⚠️  Failed to cancel existing orders: %v", err)
	}

	// Place sell order (IOC for immediate fill)
	orderID, err := t.placeOrder(tickerID, price, quantity, false, leverage, false, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to place sell order: %w", err)
	}

	logger.Infof("✓ Nado short position opened: %s qty=%.4f orderID=%s", symbol, quantity, orderID)

	return map[string]interface{}{
		"orderId": orderID,
		"symbol":  symbol,
		"status":  "FILLED",
	}, nil
}

// CloseLong closes a long position
func (t *NadoTrader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	logger.Infof("🔄 Closing Nado long position: %s", symbol)

	// If quantity is 0, get current position
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}

		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "long" {
				quantity = pos["positionAmt"].(float64)
				break
			}
		}

		if quantity == 0 {
			return nil, fmt.Errorf("no long position found for %s", symbol)
		}
	}

	// Convert symbol to Nado format
	tickerID := convertToNadoSymbol(symbol)

	// Get current price
	price, err := t.GetMarketPrice(symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get market price: %w", err)
	}

	// Place sell order with reduce_only
	orderID, err := t.placeOrder(tickerID, price, quantity, false, 0, true, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to place sell order: %w", err)
	}

	// Cancel remaining orders
	if err := t.CancelAllOrders(symbol); err != nil {
		logger.Infof("⚠️  Failed to cancel remaining orders: %v", err)
	}

	logger.Infof("✓ Nado long position closed: %s qty=%.4f orderID=%s", symbol, quantity, orderID)

	return map[string]interface{}{
		"orderId": orderID,
		"symbol":  symbol,
		"status":  "FILLED",
	}, nil
}

// CloseShort closes a short position
func (t *NadoTrader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	logger.Infof("🔄 Closing Nado short position: %s", symbol)

	// If quantity is 0, get current position
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}

		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "short" {
				quantity = pos["positionAmt"].(float64)
				break
			}
		}

		if quantity == 0 {
			return nil, fmt.Errorf("no short position found for %s", symbol)
		}
	}

	// Convert symbol to Nado format
	tickerID := convertToNadoSymbol(symbol)

	// Get current price
	price, err := t.GetMarketPrice(symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to get market price: %w", err)
	}

	// Place buy order with reduce_only
	orderID, err := t.placeOrder(tickerID, price, quantity, true, 0, true, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to place buy order: %w", err)
	}

	// Cancel remaining orders
	if err := t.CancelAllOrders(symbol); err != nil {
		logger.Infof("⚠️  Failed to cancel remaining orders: %v", err)
	}

	logger.Infof("✓ Nado short position closed: %s qty=%.4f orderID=%s", symbol, quantity, orderID)

	return map[string]interface{}{
		"orderId": orderID,
		"symbol":  symbol,
		"status":  "FILLED",
	}, nil
}

// SetLeverage sets leverage
func (t *NadoTrader) SetLeverage(symbol string, leverage int) error {
	// Nado uses unified margin, leverage is set per position via order appendix
	logger.Infof("✓ Nado leverage will be set to %dx on next order for %s", leverage, symbol)
	return nil
}

// SetMarginMode sets margin mode
func (t *NadoTrader) SetMarginMode(symbol string, isCrossMargin bool) error {
	// Nado uses unified margin by default
	logger.Infof("✓ Nado uses unified margin mode")
	return nil
}

// GetMarketPrice gets market price
func (t *NadoTrader) GetMarketPrice(symbol string) (float64, error) {
	// Convert symbol to Nado format
	tickerID := convertToNadoSymbol(symbol)

	// Query orderbook to get best price
	data, err := t.httpClient.get("/orderbook?ticker_id=" + tickerID + "&depth=1")
	if err != nil {
		return 0, fmt.Errorf("failed to get orderbook: %w", err)
	}

	var orderbook struct {
		Bids []struct {
			Price string `json:"price"`
		} `json:"bids"`
		Asks []struct {
			Price string `json:"price"`
		} `json:"asks"`
	}

	if err := json.Unmarshal(data, &orderbook); err != nil {
		return 0, fmt.Errorf("failed to parse orderbook: %w", err)
	}

	// Use mid price
	if len(orderbook.Bids) > 0 && len(orderbook.Asks) > 0 {
		bidPrice, _ := strconv.ParseFloat(orderbook.Bids[0].Price, 64)
		askPrice, _ := strconv.ParseFloat(orderbook.Asks[0].Price, 64)
		return (bidPrice + askPrice) / 2, nil
	}

	return 0, fmt.Errorf("no market data available for %s", symbol)
}

// SetStopLoss sets stop loss order
func (t *NadoTrader) SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error {
	logger.Infof("🛑 Setting Nado stop loss: %s %s price=%.4f", symbol, positionSide, stopPrice)

	tickerID := convertToNadoSymbol(symbol)
	isBuy := (positionSide == "SHORT") // Short position SL = buy, long SL = sell

	_, err := t.placeOrder(tickerID, stopPrice, quantity, isBuy, 0, true, 1) // trigger type = 1 (PRICE)
	if err != nil {
		return fmt.Errorf("failed to set stop loss: %w", err)
	}

	logger.Infof("✓ Stop loss set at %.4f", stopPrice)
	return nil
}

// SetTakeProfit sets take profit order
func (t *NadoTrader) SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error {
	logger.Infof("🎯 Setting Nado take profit: %s %s price=%.4f", symbol, positionSide, takeProfitPrice)

	tickerID := convertToNadoSymbol(symbol)
	isBuy := (positionSide == "SHORT") // Short position TP = buy, long TP = sell

	_, err := t.placeOrder(tickerID, takeProfitPrice, quantity, isBuy, 0, true, 1) // trigger type = 1 (PRICE)
	if err != nil {
		return fmt.Errorf("failed to set take profit: %w", err)
	}

	logger.Infof("✓ Take profit set at %.4f", takeProfitPrice)
	return nil
}

// CancelStopLossOrders cancels stop loss orders
func (t *NadoTrader) CancelStopLossOrders(symbol string) error {
	// Nado doesn't distinguish SL/TP in order list, need to cancel all trigger orders
	return t.CancelStopOrders(symbol)
}

// CancelTakeProfitOrders cancels take profit orders
func (t *NadoTrader) CancelTakeProfitOrders(symbol string) error {
	// Nado doesn't distinguish SL/TP in order list, need to cancel all trigger orders
	return t.CancelStopOrders(symbol)
}

// CancelAllOrders cancels all orders for a symbol
func (t *NadoTrader) CancelAllOrders(symbol string) error {
	logger.Infof("🚫 Cancelling all Nado orders for %s", symbol)

	// Get open orders
	data, err := t.query("orders", map[string]string{
		"subaccount": t.walletAddr.Hex(),
	})
	if err != nil {
		return fmt.Errorf("failed to get orders: %w", err)
	}

	var orders []struct {
		OrderID string `json:"order_id"`
		TickerID string `json:"ticker_id"`
	}
	if err := json.Unmarshal(data, &orders); err != nil {
		return fmt.Errorf("failed to parse orders: %w", err)
	}

	tickerID := convertToNadoSymbol(symbol)

	// Cancel each order for this symbol
	for _, order := range orders {
		if order.TickerID == tickerID {
			if err := t.cancelOrder(order.OrderID); err != nil {
				logger.Infof("⚠️  Failed to cancel order %s: %v", order.OrderID, err)
			}
		}
	}

	logger.Infof("✓ Cancelled all orders for %s", symbol)
	return nil
}

// CancelStopOrders cancels stop orders (SL/TP)
func (t *NadoTrader) CancelStopOrders(symbol string) error {
	return t.CancelAllOrders(symbol)
}

// FormatQuantity formats quantity to correct precision
func (t *NadoTrader) FormatQuantity(symbol string, quantity float64) (string, error) {
	// Get pair info to determine precision
	tickerID := convertToNadoSymbol(symbol)

	t.metaMutex.RLock()
	defer t.metaMutex.RUnlock()

	if t.meta == nil {
		return fmt.Sprintf("%.4f", quantity), nil
	}

	for _, pair := range t.meta.Pairs {
		if pair.TickerID == tickerID {
			// Use tick size for precision
			tickSize, _ := strconv.ParseFloat(pair.TickSize, 64)
			precision := getPrecision(tickSize)
			formatStr := fmt.Sprintf("%%.%df", precision)
			return fmt.Sprintf(formatStr, quantity), nil
		}
	}

	return fmt.Sprintf("%.4f", quantity), nil
}

// GetOrderStatus gets order status
func (t *NadoTrader) GetOrderStatus(symbol string, orderID string) (map[string]interface{}, error) {
	data, err := t.query("orders", map[string]string{
		"subaccount": t.walletAddr.Hex(),
		"order_id":   orderID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get order status: %w", err)
	}

	var order struct {
		Status      string `json:"status"` // "open", "filled", "cancelled"
		AvgPrice    string `json:"avg_price"`
		FilledQty   string `json:"filled_qty"`
		Fee         string `json:"fee"`
	}

	if err := json.Unmarshal(data, &order); err != nil {
		return nil, fmt.Errorf("failed to parse order status: %w", err)
	}

	avgPrice, _ := strconv.ParseFloat(order.AvgPrice, 64)
	filledQty, _ := strconv.ParseFloat(order.FilledQty, 64)
	fee, _ := strconv.ParseFloat(order.Fee, 64)

	return map[string]interface{}{
		"orderId":     orderID,
		"status":      strings.ToUpper(order.Status),
		"avgPrice":    avgPrice,
		"executedQty": filledQty,
		"commission":  fee,
	}, nil
}

// GetClosedPnL gets closed position PnL records
func (t *NadoTrader) GetClosedPnL(startTime time.Time, limit int) ([]ClosedPnLRecord, error) {
	// Query trade history from archive
	startTimeMs := startTime.UnixMilli()

	data, err := t.archiveGet("/trades", map[string]string{
		"subaccount":    t.walletAddr.Hex(),
		"start_time":    strconv.FormatInt(startTimeMs, 10),
		"limit":         strconv.Itoa(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get trades: %w", err)
	}

	var trades []struct {
		TradeID      string `json:"trade_id"`
		TickerID     string `json:"ticker_id"`
		Price        string `json:"price"`
		BaseFilled   string `json:"base_filled"`
		QuoteFilled  string `json:"quote_filled"`
		Timestamp    int64  `json:"timestamp"`
		TradeType    string `json:"trade_type"` // "buy" or "sell"
		Fee          string `json:"fee"`
		ClosedPnL    string `json:"closed_pnl"` // Only for closing trades
	}

	if err := json.Unmarshal(data, &trades); err != nil {
		return nil, fmt.Errorf("failed to parse trades: %w", err)
	}

	var records []ClosedPnLRecord
	for _, trade := range trades {
		pnl, _ := strconv.ParseFloat(trade.ClosedPnL, 64)
		if pnl == 0 {
			continue // Skip opening trades
		}

		price, _ := strconv.ParseFloat(trade.Price, 64)
		qty, _ := strconv.ParseFloat(trade.BaseFilled, 64)
		fee, _ := strconv.ParseFloat(trade.Fee, 64)

		symbol := normalizeNadoSymbol(trade.TickerID)
		side := "long"
		if trade.TradeType == "sell" {
			side = "long" // Selling closes long
		} else {
			side = "short" // Buying closes short
		}

		records = append(records, ClosedPnLRecord{
			Symbol:      symbol,
			Side:        side,
			ExitPrice:   price,
			Quantity:    qty,
			RealizedPnL: pnl,
			Fee:         fee,
			ExitTime:    time.Unix(trade.Timestamp, 0),
			EntryTime:   time.Unix(trade.Timestamp, 0), // Nado doesn't provide entry time
			OrderID:     trade.TradeID,
			ExchangeID:  trade.TradeID,
			CloseType:   "unknown",
		})
	}

	return records, nil
}

// placeOrder places an order on Nado
func (t *NadoTrader) placeOrder(tickerID string, price, quantity float64, isBuy bool, leverage int, reduceOnly bool, triggerType int) (string, error) {
	// Generate nonce
	nonce := t.generateNonce()

	// Build appendix
	// Format: version(8) + isolated(1) + orderType(2) + reduceOnly(1) + triggerType(2) + value(64)
	appendix := buildAppendix(leverage, reduceOnly, triggerType)

	// Convert price to x18 format
	priceX18 := price * 1e18

	// Convert amount to 18 decimals
	amount := quantity * 1e18

	// Build order request
	order := NadoOrderRequest{
		Sender:     formatSender(t.walletAddr),
		PriceX18:   formatInt256(int64(priceX18)),
		Amount:     formatInt256(int64(amount)),
		Expiration: "4294967295", // Max uint32 (no expiration)
		Nonce:      formatUint128(nonce),
		Appendix:   formatUint128(appendix),
	}

	// Sign order
	signature, err := t.signOrder(&order)
	if err != nil {
		return "", fmt.Errorf("failed to sign order: %w", err)
	}

	// Get product ID for ticker
	productID, err := t.getProductID(tickerID)
	if err != nil {
		return "", err
	}

	// Build execute request
	execReq := map[string]interface{}{
		"place_order": map[string]interface{}{
			"product_id": productID,
			"order":      order,
			"signature":  signature,
			"id":         nonce,
		},
	}

	// Execute order
	respData, err := t.execute(execReq)
	if err != nil {
		return "", fmt.Errorf("execute failed: %w", err)
	}

	var resp NadoExecuteResponse
	if err := json.Unmarshal(respData, &resp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.Error != nil {
		return "", fmt.Errorf("order error (code %d): %s", resp.Error.Code, resp.Error.Message)
	}

	return strconv.FormatUint(nonce, 10), nil
}

// cancelOrder cancels an order
func (t *NadoTrader) cancelOrder(orderID string) error {
	nonce := t.generateNonce()

	cancelReq := map[string]interface{}{
		"cancel_order": map[string]interface{}{
			"order_id": orderID,
			"signature": "0x", // Cancellation doesn't require signature
			"id":       nonce,
		},
	}

	_, err := t.execute(cancelReq)
	return err
}

// execute executes an action via Nado API
func (t *NadoTrader) execute(req map[string]interface{}) ([]byte, error) {
	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	resp, err := t.httpClient.post("/execute", jsonData)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// query queries data via Nado API
func (t *NadoTrader) query(queryType string, params map[string]string) ([]byte, error) {
	values := map[string]string{"type": queryType}
	for k, v := range params {
		values[k] = v
	}

	// Build query string
	var queryParts []string
	for k, v := range values {
		queryParts = append(queryParts, k+"="+v)
	}
	queryStr := strings.Join(queryParts, "&")

	return t.httpClient.get("/query?" + queryStr)
}

// archiveGet queries archive API
func (t *NadoTrader) archiveGet(endpoint string, params map[string]string) ([]byte, error) {
	return t.archiveGetWithClient(endpoint, params)
}

// generateNonce generates a unique nonce
func (t *NadoTrader) generateNonce() uint64 {
	t.nonceMutex.Lock()
	defer t.nonceMutex.Unlock()

	now := time.Now().UnixNano()
	if now <= t.lastNonce {
		now = t.lastNonce + 1
	}
	t.lastNonce = now

	return uint64(now)
}

// signOrder signs an order using EIP-712
func (t *NadoTrader) signOrder(order *NadoOrderRequest) (string, error) {
	// EIP-712 typed data
	typedData := apitypes.TypedData{
	 Types: apitypes.Types{
		 "Order": []apitypes.Type{
				{Name: "sender", Type: "bytes32"},
				{Name: "priceX18", Type: "int256"},
				{Name: "amount", Type: "int256"},
				{Name: "expiration", Type: "uint64"},
				{Name: "nonce", Type: "uint128"},
				{Name: "appendix", Type: "uint128"},
			},
		},
		PrimaryType: "Order",
		Domain: apitypes.TypedDataDomain{
			Name:              "Nado",
			Version:           "0.0.1",
			ChainId:           math.NewHexOrDecimal256(33069), // Ink Layer2 mainnet
			VerifyingContract: "0x0000000000000000000000000000000000000000", // Placeholder
		},
		Message: apitypes.TypedDataMessage{
			"sender":     order.Sender,
			"priceX18":   order.PriceX18,
			"amount":     order.Amount,
			"expiration": order.Expiration,
			"nonce":      order.Nonce,
			"appendix":   order.Appendix,
		},
	}

	// Adjust chain ID for testnet
	if t.testnet {
		typedData.Domain.ChainId = math.NewHexOrDecimal256(40512) // Ink Layer2 testnet
	}

	// Sign
	hash, err := signerHash(typedData)
	if err != nil {
		return "", err
	}

	signature, err := crypto.Sign(hash, t.privateKey)
	if err != nil {
		return "", err
	}

	return "0x" + hex.EncodeToString(signature), nil
}

// getProductID gets product ID for ticker
func (t *NadoTrader) getProductID(tickerID string) (int, error) {
	t.metaMutex.RLock()
	defer t.metaMutex.RUnlock()

	if t.meta == nil {
		return 0, fmt.Errorf("metadata not loaded")
	}

	for _, pair := range t.meta.Pairs {
		if pair.TickerID == tickerID {
			return pair.ProductID, nil
		}
	}

	return 0, fmt.Errorf("product not found for ticker %s", tickerID)
}

// Helper functions

func normalizeNadoSymbol(tickerID string) string {
	// Convert "BTC-PERP_USDT0" to "BTCUSDT"
	parts := strings.Split(tickerID, "_")
	if len(parts) >= 2 {
		base := strings.TrimSuffix(parts[0], "-PERP")
		quote := strings.TrimSuffix(parts[1], "0")
		return base + quote
	}
	return tickerID
}

func convertToNadoSymbol(symbol string) string {
	// Convert "BTCUSDT" to "BTC-PERP_USDT0"
	if len(symbol) >= 4 && symbol[len(symbol)-4:] == "USDT" {
		base := symbol[:len(symbol)-4]
		return base + "-PERP_USDT0"
	}
	return symbol
}

func formatSender(addr common.Address) string {
	// Pad address to 32 bytes (66 chars with 0x)
	return "0x" + addr.Hex()[2:] + strings.Repeat("0", 64-40) + "73743000000000000000"
}

func formatInt256(value int64) string {
	if value < 0 {
		return "-" + formatUint256(uint64(-value))
	}
	return formatUint256(uint64(value))
}

func formatUint256(value uint64) string {
	return fmt.Sprintf("%064x", value)
}

func formatUint128(value uint64) string {
	return fmt.Sprintf("%016x", value)
}

func buildAppendix(leverage int, reduceOnly bool, triggerType int) uint64 {
	// Build appendix bitfield
	// [version(8)] [isolated(1)] [orderType(2)] [reduceOnly(1)] [triggerType(2)] [value(50)]
	appendix := uint64(1 << 56) // version = 1

	if reduceOnly {
		appendix |= 1 << 53
	}

	if triggerType > 0 {
		appendix |= uint64(triggerType&0x3) << 51
	}

	// Store leverage in value field (0-20 fits in 50 bits)
	if leverage > 0 && leverage <= 20 {
		appendix |= uint64(leverage)
	}

	return appendix
}

func getPrecision(value float64) int {
	precision := 0
	for value < 1 {
		value *= 10
		precision++
	}
	return precision
}

// HTTP client methods

func (c *httpClient) get(path string) ([]byte, error) {
	url := c.baseURL + path

	client := &http.Client{Timeout: c.timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *httpClient) post(path string, data []byte) ([]byte, error) {
	url := c.baseURL + path

	client := &http.Client{Timeout: c.timeout}
	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("HTTP POST failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// archiveGetWithClient performs GET request to archive endpoint
func (t *NadoTrader) archiveGetWithClient(endpoint string, params map[string]string) ([]byte, error) {
	// Build query string
	var queryParts []string
	for k, v := range params {
		queryParts = append(queryParts, k+"="+v)
	}
	queryStr := strings.Join(queryParts, "&")

	url := t.archiveURL + endpoint
	if queryStr != "" {
		url += "?" + queryStr
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("archive HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read archive response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("archive HTTP error %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// signerHash creates EIP-712 hash
func signerHash(typedData apitypes.TypedData) ([]byte, error) {
	typedData.Domain.Salt = nil
	return apitypes.TypedDataAndHashToEIP712(typedData)
}
