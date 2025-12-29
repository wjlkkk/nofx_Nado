# Nado DEX Integration Guide

This document explains how to use the Nado DEX integration in NOFX.

## Overview

Nado is a central-limit orderbook DEX built on Ink Layer2, backed by the Kraken team. It supports:
- Spot and perpetual trading
- Unified margin across all positions
- Up to 20x leverage
- Low fees (1.5 bps taker, maker rebates)
- Self-custodial trading

## Configuration

### Environment Variables

To use Nado in your trader configuration, you need to provide:

```go
config := trader.AutoTraderConfig{
    // ... other config ...

    // Exchange selection
    Exchange: "nado",

    // Nado configuration
    NadoPrivateKey: "0x...",  // Your wallet private key (64 hex chars)
    NadoTestnet:    false,     // Set to true for testnet
}
```

### Frontend Configuration

When creating a trader through the web UI, select:
- **Exchange**: Nado
- **Private Key**: Your Ethereum private key for Nado wallet
- **Testnet**: Check box for testnet trading

## API Endpoints

### Mainnet
- Gateway: `https://gateway.prod.nado.xyz/v1`
- Archive: `https://archive.prod.nado.xyz/v1`
- WebSocket: `wss://gateway.prod.nado.xyz/v1/ws`

### Testnet
- Gateway: `https://gateway.test.nado.xyz/v1`
- Archive: `https://archive.test.nado.xyz/v1`
- WebSocket: `wss://gateway.test.nado.xyz/v1/ws`

## Supported Features

### ✅ Implemented
- Account balance queries
- Position management (open/close long/short)
- Market orders (IOC for immediate execution)
- Limit orders (stop-loss, take-profit)
- Order cancellation
- Order and position synchronization
- Trade history retrieval
- Leverage setting (1-20x)
- Unified margin mode

### ⚠️ Limitations
- No native Go SDK (using REST API)
- EIP-712 signing required for all orders
- No WebSocket streaming (polling-based)
- Order sync runs every 30 seconds

## Symbol Format

Nado uses a special ticker format:
- **Nado format**: `BTC-PERP_USDT0`
- **Standard format**: `BTCUSDT`

The integration automatically converts between formats:
```
BTCUSDT (NOFX) → BTC-PERP_USDT0 (Nado)
BTC-PERP_USDT0 (Nado) → BTCUSDT (NOFX)
```

## Authentication

Nado uses EIP-712 typed data signing for all execute operations:

```go
// Order structure
type Order struct {
    Sender     string  // 32 bytes (padded address)
    PriceX18   string  // Price in 1e18 format
    Amount     string  // Amount in 1e18 format
    Expiration string  // Unix timestamp
    Nonce      string  // uint128
    Appendix   string  // uint128 (encoding order params)
}
```

### Appendix Encoding

The appendix field encodes order parameters:
- **Version** (8 bits): Protocol version
- **Isolated** (1 bit): Margin mode
- **Order Type** (2 bits): DEFAULT, IOC, FOK, POST_ONLY
- **Reduce Only** (1 bit): Closing order only
- **Trigger Type** (2 bits): NONE, PRICE, TWAP
- **Value** (64 bits): Leverage or TWAP params

## Example Usage

### Creating a Nado Trader

```go
import "nofx/trader"

// Initialize trader
config := trader.AutoTraderConfig{
    ID:            "nado_trader_1",
    Name:          "Nado BTC Trader",
    Exchange:      "nado",
    NadoPrivateKey: "0x1234567890abcdef...", // Your private key
    NadoTestnet:   false,

    // Strategy configuration
    StrategyConfig: strategyConfig,

    // Risk management
    InitialBalance: 1000.0,
    ScanInterval:   3 * time.Minute,
}

// Create trader
autoTrader, err := trader.NewAutoTrader(config, store, userID)
if err != nil {
    log.Fatal(err)
}

// Start trading
go autoTrader.Run()
```

### Manual Trading

```go
// Create trader directly
nadoTrader, err := trader.NewNadoTrader(privateKey, false)
if err != nil {
    log.Fatal(err)
}

// Get balance
balance, err := nadoTrader.GetBalance()

// Get positions
positions, err := nadoTrader.GetPositions()

// Open long position
order, err := nadoTrader.OpenLong("BTCUSDT", 0.01, 5) // 0.01 BTC, 5x leverage

// Close position
order, err := nadoTrader.CloseLong("BTCUSDT", 0)

// Set stop loss
err := nadoTrader.SetStopLoss("BTCUSDT", "LONG", 0.01, 95000)

// Set take profit
err := nadoTrader.SetTakeProfit("BTCUSDT", "LONG", 0.01, 105000)
```

## Order Synchronization

The Nado integration includes automatic order and position synchronization:

- **Sync Interval**: Every 30 seconds
- **Trades Synced**: All fills from exchange
- **Positions Synced**: Current open positions
- **Auto-Close**: Positions closed externally detected

### Sync Data Flow

```
Nado Exchange (Archive API)
    ↓ GetTrades()
Trade Records
    ↓ processTrade()
Database (trader_orders, trader_fills)
    ↓ ProcessTrade()
Database (trader_positions)
```

## Testing

Run the Nado trader tests:

```bash
cd trader
go test -v -run TestNewNadoTrader
go test -v -run TestNormalizeNadoSymbol
go test -v -run TestConvertToNadoSymbol
```

## Security Considerations

1. **Private Key Security**
   - Never commit private keys to git
   - Use environment variables or encrypted storage
   - Consider using a separate agent wallet

2. **Testnet First**
   - Always test on testnet before mainnet
   - Verify order execution and position management
   - Check sync functionality

3. **Risk Management**
   - Start with small position sizes
   - Use stop-loss orders
   - Monitor position sizes closely
   - Keep leverage conservative (1-5x recommended)

## Troubleshooting

### Common Issues

**"metadata not loaded"**
- Solution: The trader will retry metadata fetch on next operation

**"failed to sign order"**
- Solution: Check private key format (64 hex chars, with or without 0x prefix)

**"product not found"**
- Solution: Verify symbol format (BTCUSDT, not BTC/USDT)

**"HTTP error"**
- Solution: Check network connectivity and API endpoint status

### Debug Logging

Enable debug logging by setting log level:

```go
logger.SetLevel("debug")
```

This will show:
- API request/response details
- Order signing process
- Sync operations
- Error details

## Resources

- **Nado Docs**: https://docs.nado.xyz/
- **Nado Website**: https://www.nado.xyz/
- **Ink Layer2**: https://ink.onthebridge.com/
- **API Reference**: https://docs.nado.xyz/developer-resources/api/
- **Python SDK**: https://nadohq.github.io/nado-python-sdk/

## Changelog

### v1.0.0 (2025-01-XX)
- Initial Nado DEX integration
- REST API client
- EIP-712 order signing
- Order and position sync
- Basic trading operations
- Unit tests

---

**Note**: This integration uses the Nado REST API directly as there is no official Go SDK. For production use, consider adding retry logic, rate limiting, and error handling improvements.
