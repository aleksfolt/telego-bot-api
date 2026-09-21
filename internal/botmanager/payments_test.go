package botmanager

import (
	"testing"

	"github.com/gotd/td/tg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"telego-bot-api/internal/converter"
)

func TestConvertStarTransactionUsesBotAPIPartnerShape(t *testing.T) {
	transaction := tg.StarsTransaction{
		ID:                 "charge-1",
		Amount:             &tg.StarsAmount{Amount: 193},
		Date:               1_700_000_000,
		Peer:               &tg.StarsTransactionPeer{Peer: &tg.PeerUser{UserID: 123}},
		BotPayload:         []byte("subscribe_1m_123_456"),
		SubscriptionPeriod: 2_592_000,
	}
	transaction.SetFlags()

	converted := convertStarTransaction(transaction, map[int64]converter.User{
		123: {ID: 123, FirstName: "Buyer"},
	})
	assert.Equal(t, "charge-1", converted["id"])
	assert.Equal(t, int64(193), converted["amount"])
	require.NotContains(t, converted, "invoice_payload")

	source, ok := converted["source"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "user", source["type"])
	assert.Equal(t, "invoice_payment", source["transaction_type"])
	assert.Equal(t, "subscribe_1m_123_456", source["invoice_payload"])
	assert.Equal(t, 2_592_000, source["subscription_period"])
}

func TestConvertStarTransactionRefundUsesReceiver(t *testing.T) {
	transaction := tg.StarsTransaction{
		Refund:     true,
		ID:         "charge-1",
		Amount:     &tg.StarsAmount{Amount: 193},
		Date:       1_700_000_000,
		Peer:       &tg.StarsTransactionPeer{Peer: &tg.PeerUser{UserID: 123}},
		BotPayload: []byte("subscribe_1m_123_456"),
	}
	transaction.SetFlags()

	converted := convertStarTransaction(transaction, map[int64]converter.User{123: {ID: 123}})
	assert.NotContains(t, converted, "source")
	assert.Contains(t, converted, "receiver")
}
