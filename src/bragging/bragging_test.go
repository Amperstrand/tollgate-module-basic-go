package bragging

import (
    "context"
    "testing"

    "github.com/nbd-wtf/go-nostr"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestEventCreation(t *testing.T) {
    config := Config{Enabled: true, UserOptIn: true, Fields: []string{"amount", "mint", "duration"}, Template: "New sale! {{.amount}} sats via {{.mint}} for {{.duration}} sec"}
    privateKey := nostr.GeneratePrivateKey()
    service, err := NewService(config, privateKey)
    require.NoError(t, err)

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
    privateKey := nostr.GeneratePrivateKey()
    service, err := NewService(config, privateKey)
    require.NoError(t, err)

    output := service.renderTemplate(map[string]interface{}{
        "amount": 150,
        "mint":   "https://mint.example",
    })

    assert.Contains(t, output, "Sale: 150 @ https://mint.example")
}

func TestRelayPublish(t *testing.T) {
    relayURL := "wss://relay.damus.io"
    service := &Service{
        config:    Config{Relays: []string{relayURL}},
        relayPool: nostr.NewSimplePool(context.Background()),
    }

    event := &nostr.Event{
        Kind:      13111,
        CreatedAt: nostr.Now(),
        Tags:      make(nostr.Tags, 0),
        Content:   "Test event",
    }
    privateKey := nostr.GeneratePrivateKey()
    publicKey, err := nostr.GetPublicKey(privateKey)
    require.NoError(t, err)
    t.Logf("Public Key: %s", publicKey)

    event.Sign(privateKey)
    t.Logf("Event ID: %s", event.ID)

    err = service.PublishEvent(event)

    assert.NoError(t, err)
    // Note: Checking event reception on the relay is not straightforward without additional relay-specific logic
}