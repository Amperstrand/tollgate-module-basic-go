package bragging

import (
    "fmt"
    "github.com/nbd-wtf/go-nostr"
    "time"
)

func (s *Service) CreateEvent(saleData map[string]interface{}) (*nostr.Event, error) {
    if !s.config.Enabled || !s.config.UserOptIn {
        return nil, nil
    }

    event := &nostr.Event{
        Kind:      13111,
        CreatedAt: nostr.Now(),
        Tags:      make(nostr.Tags, 0),
        Content:   s.renderTemplate(saleData),
    }

    // Add standard tags
    event.Tags = append(event.Tags, nostr.Tag{"p", s.keyPair.PublicKey(), "BraggingTollGate"})

    // Add configured fields
    for _, field := range s.config.Fields {
        if value, exists := saleData[field]; exists {
            event.Tags = append(event.Tags, nostr.Tag{field, fmt.Sprint(value)})
        }
    }

    return event.Sign(s.keyPair.PrivateKey)
}