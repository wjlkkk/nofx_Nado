package trader

import (
	"encoding/json"
	"fmt"
	"math"
	"nofx/logger"
	"nofx/market"
	"nofx/store"
	"strconv"
	"strings"
	"sync"
	"time"
)

// StartOrderSync starts background order and position synchronization
func (t *NadoTrader) StartOrderSync(traderID, exchangeID, exchangeType string, st *store.Store, interval time.Duration) {
	if st == nil {
		logger.Infof("⚠️  Store is nil, skipping Nado order sync")
		return
	}

	sync := &NadoOrderSync{
		trader:      t,
		traderID:    traderID,
		exchangeID:  exchangeID,
		exchangeType: exchangeType,
		store:       st,
		interval:    interval,
		stopCh:      make(chan struct{}),
		wg:          sync.WaitGroup{},
	}

	sync.wg.Add(1)
	go sync.run()

	logger.Infof("🔄 Nado order sync started (interval: %v)", interval)
}

// NadoOrderSync handles order and position synchronization
type NadoOrderSync struct {
	trader       *NadoTrader
	traderID     string
	exchangeID   string
	exchangeType string
	store        *store.Store
	interval     time.Duration
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

// run runs the sync loop
func (s *NadoOrderSync) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Initial sync
	s.syncOrders()
	s.syncPositions()

	for {
		select {
		case <-ticker.C:
			s.syncOrders()
			s.syncPositions()
		case <-s.stopCh:
			logger.Infof("⏹ Nado order sync stopped")
			return
		}
	}
}

// Stop stops the sync
func (s *NadoOrderSync) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

// syncOrders syncs recent trades
func (s *NadoOrderSync) syncOrders() {
	// Get last sync time from store
	lastSyncTime, err := s.store.Order().GetLastSyncTime(s.traderID)
	if err != nil {
		lastSyncTime = time.Now().Add(-24 * time.Hour) // Default to 24h ago
	}

	// Get trades from Nado
	trades, err := s.trader.GetTrades(lastSyncTime, 100)
	if err != nil {
		logger.Infof("⚠️  Failed to get Nado trades: %v", err)
		return
	}

	if len(trades) == 0 {
		return
	}

	logger.Infof("📥 Nado: syncing %d trades since %s", len(trades), lastSyncTime.Format("15:04:05"))

	// Process each trade
	for _, trade := range trades {
		if err := s.processTrade(trade); err != nil {
			logger.Infof("⚠️  Failed to process trade %s: %v", trade.TradeID, err)
		}
	}

	// Update last sync time
	if err := s.store.Order().UpdateLastSyncTime(s.traderID, time.Now()); err != nil {
		logger.Infof("⚠️  Failed to update last sync time: %v", err)
	}
}

// syncPositions syncs current positions
func (s *NadoOrderSync) syncPositions() {
	positions, err := s.trader.GetPositions()
	if err != nil {
		logger.Infof("⚠️  Failed to get Nado positions: %v", err)
		return
	}

	// Mark all positions as potentially closed
	openPositions := make(map[string]bool)

	// Update/create positions
	for _, pos := range positions {
		symbol := pos["symbol"].(string)
		side := pos["side"].(string)
		quantity := pos["positionAmt"].(float64)
		entryPrice := pos["entryPrice"].(float64)

		if quantity == 0 {
			continue
		}

		openPositions[symbol+"_"+side] = true

		// Normalize symbol
		normalizedSymbol := market.Normalize(symbol)

		// Check if position exists in database
		dbPos, err := s.store.Position().GetOpenPositionBySymbol(s.traderID, normalizedSymbol, strings.ToUpper(side))
		if err != nil {
			// Create new position
			newPos := &store.TraderPosition{
				TraderID:     s.traderID,
				ExchangeID:   s.exchangeID,
				ExchangeType: s.exchangeType,
				Symbol:       normalizedSymbol,
				Side:         strings.ToUpper(side),
				Quantity:     quantity,
				EntryPrice:   entryPrice,
				EntryTime:    time.Now(),
				Leverage:     int(pos["leverage"].(float64)),
				Status:       "OPEN",
			}
			if err := s.store.Position().Create(newPos); err != nil {
				logger.Infof("⚠️  Failed to create position: %v", err)
			}
		} else {
			// Update existing position quantity if changed
			if math.Abs(dbPos.Quantity-quantity) > 0.000001 {
				if err := s.store.Position().UpdateQuantity(dbPos.ID, quantity); err != nil {
					logger.Infof("⚠️  Failed to update position quantity: %v", err)
				}
			}
		}
	}

	// Close positions not in exchange
	allPositions, err := s.store.Position().GetOpenPositions(s.traderID)
	if err != nil {
		return
	}

	for _, pos := range allPositions {
		key := pos.Symbol + "_" + pos.Side
		if !openPositions[key] {
			// Position closed externally
			currentPos := findPosition(positions, pos.Symbol, pos.Side)
			if currentPos != nil {
				exitPrice := currentPos["markPrice"].(float64)
				unrealizedPnl := currentPos["unRealizedProfit"].(float64)

				// Calculate realized PnL
				var realizedPnl float64
				if pos.Side == "LONG" {
					realizedPnl = (exitPrice - pos.EntryPrice) * pos.Quantity
				} else {
					realizedPnl = (pos.EntryPrice - exitPrice) * pos.Quantity
				}

				// Close position in database
				if err := s.store.Position().ClosePosition(
					pos.ID,
					exitPrice,
					realizedPnl,
					0, // Fee will be fetched from trades
					time.Now(),
					"external",
				); err != nil {
					logger.Infof("⚠️  Failed to close position: %v", err)
				} else {
					logger.Infof("✅ Position closed externally: %s %s", pos.Symbol, pos.Side)
				}
			}
		}
	}
}

// processTrade processes a single trade
func (s *NadoOrderSync) processTrade(trade TradeRecord) error {
	// Normalize symbol
	normalizedSymbol := market.Normalize(trade.Symbol)

	// Check if order already exists
	existingOrder, err := s.store.Order().GetByExchangeID(s.traderID, trade.TradeID)
	if err == nil && existingOrder != nil {
		// Order already exists, skip
		return nil
	}

	// Determine side and action
	var side string
	var action string

	switch trade.OrderAction {
	case "open_long":
		side = "BUY"
		action = "open_long"
	case "open_short":
		side = "SELL"
		action = "open_short"
	case "close_long":
		side = "SELL"
		action = "close_long"
	case "close_short":
		side = "BUY"
		action = "close_short"
	default:
		// Infer from side and realized PnL
		if trade.RealizedPnL != 0 {
			// Closing trade
			if trade.Side == "BUY" {
				side = "BUY"
				action = "close_short"
			} else {
				side = "SELL"
				action = "close_long"
			}
		} else {
			// Opening trade
			if trade.Side == "BUY" {
				side = "BUY"
				action = "open_long"
			} else {
				side = "SELL"
				action = "open_short"
			}
		}
	}

	// Create order record
	order := &store.TraderOrder{
		TraderID:        s.traderID,
		ExchangeID:      s.exchangeID,
		ExchangeType:    s.exchangeType,
		ExchangeOrderID: trade.TradeID,
		Symbol:          normalizedSymbol,
		Side:            side,
		PositionSide:    strings.ToUpper(strings.Split(action, "_")[1]), // LONG or SHORT
		Type:            "MARKET",
		TimeInForce:     "IOC",
		Quantity:        trade.Quantity,
		Price:           trade.Price,
		Status:          "FILLED",
		FilledQuantity:  trade.Quantity,
		AvgFillPrice:    trade.Price,
		Commission:      trade.Fee,
		CommissionAsset: "USDT",
		Leverage:        0, // Will be inferred from position
		ReduceOnly:      (trade.RealizedPnL != 0),
		ClosePosition:   (trade.RealizedPnL != 0),
		OrderAction:     action,
		CreatedAt:       trade.Time,
		UpdatedAt:       trade.Time,
	}

	if err := s.store.Order().CreateOrder(order); err != nil {
		return fmt.Errorf("failed to create order: %w", err)
	}

	// Create fill record
	fill := &store.TraderFill{
		TraderID:         s.traderID,
		ExchangeID:       s.exchangeID,
		ExchangeType:     s.exchangeType,
		OrderID:          order.ID,
		ExchangeOrderID:  trade.TradeID,
		ExchangeTradeID:  trade.TradeID,
		Symbol:           normalizedSymbol,
		Side:             side,
		Price:            trade.Price,
		Quantity:         trade.Quantity,
		QuoteQuantity:    trade.Price * trade.Quantity,
		Commission:       trade.Fee,
		CommissionAsset:  "USDT",
		RealizedPnL:      trade.RealizedPnL,
		IsMaker:          false,
		CreatedAt:        trade.Time,
	}

	if err := s.store.Order().CreateFill(fill); err != nil {
		return fmt.Errorf("failed to create fill: %w", err)
	}

	// Process position change
	if err := s.processPositionChange(trade, normalizedSymbol, side, action); err != nil {
		return fmt.Errorf("failed to process position change: %w", err)
	}

	return nil
}

// processPositionChange processes position change from trade
func (s *NadoOrderSync) processPositionChange(trade TradeRecord, symbol, side, action string) error {
	var positionSide string
	switch action {
	case "open_long", "close_long":
		positionSide = "LONG"
	case "open_short", "close_short":
		positionSide = "SHORT"
	}

	posBuilder := store.NewPositionBuilder(s.store.Position())
	err := posBuilder.ProcessTrade(
		s.traderID,
		s.exchangeID,
		s.exchangeType,
		symbol,
		positionSide,
		action,
		trade.Quantity,
		trade.Price,
		trade.Fee,
		trade.RealizedPnL,
		trade.Time,
		trade.TradeID,
	)

	return err
}

// GetTrades retrieves trade history from Nado
func (t *NadoTrader) GetTrades(startTime time.Time, limit int) ([]TradeRecord, error) {
	// Query archive API for trades
	startTimeMs := startTime.UnixMilli()

	data, err := t.archiveGet("/trades", map[string]string{
		"start_time": strconv.FormatInt(startTimeMs, 10),
		"limit":      strconv.Itoa(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get trades: %w", err)
	}

	var tradesResponse []struct {
		TradeID     string `json:"trade_id"`
		TickerID    string `json:"ticker_id"`
		Price       string `json:"price"`
		BaseFilled  string `json:"base_filled"`
		QuoteFilled string `json:"quote_filled"`
		Timestamp   int64  `json:"timestamp"`
		TradeType   string `json:"trade_type"` // "buy" or "sell"
		Fee         string `json:"fee"`
		ClosedPnL   string `json:"closed_pnl"` // Only for closing trades
	}

	if err := json.Unmarshal(data, &tradesResponse); err != nil {
		return nil, fmt.Errorf("failed to parse trades: %w", err)
	}

	var trades []TradeRecord
	for _, trade := range tradesResponse {
		price, _ := strconv.ParseFloat(trade.Price, 64)
		qty, _ := strconv.ParseFloat(trade.BaseFilled, 64)
		fee, _ := strconv.ParseFloat(trade.Fee, 64)
		pnl, _ := strconv.ParseFloat(trade.ClosedPnL, 64)

		// Determine side
		var side string
		if trade.TradeType == "buy" || trade.TradeType == "Buy" {
			side = "BUY"
		} else {
			side = "SELL"
		}

		// Determine order action from side and pnl
		var orderAction string
		if pnl != 0 {
			// Closing trade
			if side == "BUY" {
				orderAction = "close_short"
			} else {
				orderAction = "close_long"
			}
		} else {
			// Opening trade
			if side == "BUY" {
				orderAction = "open_long"
			} else {
				orderAction = "open_short"
			}
		}

		// Normalize symbol
		symbol := normalizeNadoSymbol(trade.TickerID)

		trades = append(trades, TradeRecord{
			TradeID:      trade.TradeID,
			Symbol:       symbol,
			Side:         side,
			PositionSide: "BOTH", // Nado doesn't have hedge mode
			OrderAction:  orderAction,
			Price:        price,
			Quantity:     qty,
			RealizedPnL:  pnl,
			Fee:          fee,
			Time:         time.UnixMilli(trade.Timestamp),
		})
	}

	return trades, nil
}

// findPosition finds a position in the list
func findPosition(positions []map[string]interface{}, symbol, side string) map[string]interface{} {
	for _, pos := range positions {
		if pos["symbol"] == symbol && pos["side"] == side {
			return pos
		}
	}
	return nil
}
