package seckill

import "testing"

func TestStockTokenEncodeParse(t *testing.T) {
	token := StockToken{
		OrderID: 101,
		GoodsID: 9,
		Amount:  2,
	}

	parsed, err := ParseStockToken(token.Encode())
	if err != nil {
		t.Fatalf("ParseStockToken() error = %v", err)
	}
	if parsed != token {
		t.Fatalf("ParseStockToken() = %+v, want %+v", parsed, token)
	}
}

func TestParseStockTokenRejectsBadValue(t *testing.T) {
	if _, err := ParseStockToken("1:2:0"); err == nil {
		t.Fatal("expected error for invalid amount")
	}
	if _, err := ParseStockToken("broken"); err == nil {
		t.Fatal("expected error for malformed token")
	}
}
