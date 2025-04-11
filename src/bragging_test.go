package bragging

import (
    "context"
    "testing"

    "github.com/nbd-wtf/go-nostr"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestEventCreation(t *testing.T) {
    config := Config{Enabled: true, Fields: []string{"amount", "mint", "duration"}, Template: "New sale! {amount} sats via {mint} for {duration} sec"}
    service, _ := NewService(config, "replace_with_test_private_key")

    saleData := map[string]interface{}{
        "amount":   150,
        "mint":     "https://mint.example",
        "duration": 900,
    }

    event, err := service.CreateEvent(saleData)
    require.NoError(t, err)
    assert.Equal(t, 13111, event.Kind)
    assert.Contains(t, event.Content, "New sale! 150 sats via https://mint.example for 900 sec")
    assert.Len(t, event.Tags, 4) // incl. p, amount, mint, duration
}

func TestTemplateRendering(t *testing.T) {
    config := Config{Template: "Sale: {{.amount}} @ {{.mint}}"}
    service, _ := NewService(config, "replace_with_test_private_key")

    output := service.renderTemplate(map[string]interface{}{
        "amount": 150,
        "mint":   "https://mint.example",
    })

    assert.Contains(t, output, "Sale: 150 @ https://mint.example")
}

func TestRelayPublish(t *testing.T) {
    mockRelay := nostr.NewMockRelay()
    service := &Service{
        config:    Config{Relays: []string{"mock_relay"}},
        relayPool: nostr.NewSimplePool(context.Background()),
    }
    service.relayPool.AddRelay(mockRelay)

    event := &nostr.Event{}
    err := service.PublishEvent(event)

    assert.NoError(t, err)
    assert.True(t, mockRelay.HasEvent(event.ID))
}