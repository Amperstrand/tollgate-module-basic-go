package bragging

import (
    "context"
    "github.com/nbd-wtf/go-nostr"
    "log"
)

type Service struct {
    config    Config
    keyPair   *nostr.KeyPair
    relayPool *nostr.SimplePool
}

type Config struct {
    Enabled    bool
    Relays     []string
    Fields     []string
    Template   string
    UserOptIn  bool
}

func NewService(config Config, privateKey string) (*Service, error) {
    keyPair := nostr.KeyPair{PrivateKey: privateKey}
    return &Service{
        config:    config,
        keyPair:   &keyPair,
        relayPool: nostr.NewSimplePool(context.Background()),
    }, nil
}