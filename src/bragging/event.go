    package bragging

import (
    "fmt"
    "github.com/nbd-wtf/go-nostr"
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
    event.Tags = append(event.Tags, nostr.Tag{"p", s.publicKey, "BraggingTollGate"})

    // Add configured fields
    for _, field := range s.config.Fields {
        if value, exists := saleData[field]; exists {
            event.Tags = append(event.Tags, nostr.Tag{field, fmt.Sprint(value)})
        }
    }

    event.Sign(s.privateKey)
    return event, nil
}