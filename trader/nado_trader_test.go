package trader

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// TestNewNadoTrader tests Nado trader initialization
func TestNewNadoTrader(t *testing.T) {
	tests := []struct {
		name        string
		privateKey  string
		testnet     bool
		expectError bool
	}{
		{
			name:        "Valid mainnet initialization",
			privateKey:  "0x" + strings.Repeat("a", 64), // Dummy 64-char hex key
			testnet:     false,
			expectError: false,
		},
		{
			name:        "Valid testnet initialization",
			privateKey:  "0x" + strings.Repeat("b", 64),
			testnet:     true,
			expectError: false,
		},
		{
			name:        "Invalid private key (too short)",
			privateKey:  "0x" + strings.Repeat("c", 32),
			testnet:     false,
			expectError: true,
		},
		{
			name:        "Invalid private key (no 0x prefix)",
			privateKey:  strings.Repeat("d", 64),
			testnet:     false,
			expectError: false, // Should accept without 0x prefix
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trader, err := NewNadoTrader(tt.privateKey, tt.testnet)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if trader == nil {
				t.Errorf("Expected trader but got nil")
				return
			}

			if trader.testnet != tt.testnet {
				t.Errorf("Expected testnet=%v, got %v", tt.testnet, trader.testnet)
			}
		})
	}
}

// TestNormalizeNadoSymbol tests symbol normalization
func TestNormalizeNadoSymbol(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"BTC-PERP_USDT0", "BTCUSDT"},
		{"ETH-PERP_USDT0", "ETHUSDT"},
		{"SOL-PERP_USDT0", "SOLUSDT"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeNadoSymbol(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeNadoSymbol(%s) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}

// TestConvertToNadoSymbol tests symbol conversion to Nado format
func TestConvertToNadoSymbol(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"BTCUSDT", "BTC-PERP_USDT0"},
		{"ETHUSDT", "ETH-PERP_USDT0"},
		{"SOLUSDT", "SOL-PERP_USDT0"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := convertToNadoSymbol(tt.input)
			if result != tt.expected {
				t.Errorf("convertToNadoSymbol(%s) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}

// TestBuildAppendix tests appendix building
func TestBuildAppendix(t *testing.T) {
	tests := []struct {
		name         string
		leverage     int
		reduceOnly   bool
		triggerType  int
		expectZero   bool
	}{
		{
			name:        "Standard order",
			leverage:    5,
			reduceOnly:  false,
			triggerType: 0,
			expectZero:  false,
		},
		{
			name:        "Reduce only order",
			leverage:    0,
			reduceOnly:  true,
			triggerType: 0,
			expectZero:  false,
		},
		{
			name:        "Trigger order",
			leverage:    10,
			reduceOnly:  true,
			triggerType: 1,
			expectZero:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildAppendix(tt.leverage, tt.reduceOnly, tt.triggerType)

			if tt.expectZero && result != 0 {
				t.Errorf("Expected appendix=0, got %d", result)
			}

			if !tt.expectZero && result == 0 {
				t.Errorf("Expected non-zero appendix, got 0")
			}
		})
	}
}

// TestFormatSender tests sender address formatting
func TestFormatSender(t *testing.T) {
	// This is a basic smoke test
	input := "0x7a5ec2748e9065794491a8d29dcf3f9edb8d7c43"
	result := formatSender(common.HexToAddress(input))

	if len(result) != 66 { // 0x + 64 hex chars
		t.Errorf("Expected 66-char result, got %d chars", len(result))
	}

	if result[:2] != "0x" {
		t.Errorf("Expected result to start with 0x, got %s", result[:2])
	}
}


