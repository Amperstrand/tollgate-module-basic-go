package bragging

import (
    "context"
    "testing"
    "time"

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
        Kind:      1,
        CreatedAt: nostr.Now(),
        Tags:      make(nostr.Tags, 0),
        Content:   "Test event",
    }
    privateKey := nostr.GeneratePrivateKey()
    publicKey, err := nostr.GetPublicKey(privateKey)
    require.NoError(t, err)
    t.Logf("Public Key: %s", publicKey)
    t.Logf("Signer nsec: %s", privateKey)

    event.Sign(privateKey)
    t.Logf("Event ID: %s", event.ID)


    err = service.PublishEvent(event)
    t.Logf("Published event to relay: %s", relayURL)
    require.NoError(t, err)

    // Fetch the event from the relay to verify it's stored
    filter := nostr.Filter{
        IDs: []string{event.ID},
    }
    relay, err := service.relayPool.EnsureRelay(relayURL)
    require.NoError(t, err)
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

    defer cancel()
    events, err := relay.QuerySync(ctx, filter)
    require.NoError(t, err)
    require.NotEmpty(t, events, "event not found on relay")
    assert.Equal(t, event.ID, events[0].ID)
    t.Logf("Fetched event from relay: %+v", events[0])
}