package merchant

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/OpenTollGate/tollgate-module-basic-go/src/config_manager"
	"github.com/sirupsen/logrus"
)

const (
	defaultRecoveryThreshold uint8 = 3
	probeTimeout                   = 30 * time.Second
	probeInterval                  = 5 * time.Minute

	// Aggressive retry: when no mints are reachable at startup (e.g. WiFi STA
	// not yet connected), probe every 15s with immediate recovery (threshold=1)
	// for up to 5 minutes. This complements the OpenWrt hotplug script that
	// restarts tollgate when the wwan interface comes up.
	aggressiveProbeInterval = 15 * time.Second
	aggressiveProbeTimeout  = 10 * time.Second
	aggressiveDuration      = 5 * time.Minute

	// Initial probe retry: if all mints fail on the first attempt, retry up to
	// 3 full rounds with a 5s delay between rounds. This handles slow DNS
	// resolution or WiFi STA not-yet-connected scenarios on OpenWrt.
	initialProbeMaxRetries = 3
	initialProbeRetryDelay = 5 * time.Second

	// CA cert bundle path on OpenWrt
	caCertBundlePath = "/etc/ssl/certs/ca-certificates.crt"
)

// logger is the package-level logrus logger for mint health tracking.
var logger = logrus.WithField("component", "mint_health_tracker")

// loadCACertPool loads the system CA cert pool, falling back to an explicit
// file load from /etc/ssl/certs/ca-certificates.crt on OpenWrt where
// x509.SystemCertPool() may return nil or empty.
func loadCACertPool() *x509.CertPool {
	// Try system cert pool first
	pool, err := x509.SystemCertPool()
	if err == nil && pool != nil && len(pool.Subjects()) > 0 {
		logger.Debugf("loadCACertPool: system cert pool loaded with %d subjects", len(pool.Subjects()))
		return pool
	}

	logger.Warnf("loadCACertPool: system cert pool unavailable or empty (err=%v), falling back to %s", err, caCertBundlePath)

	// Fallback: manually load from the OpenWrt CA bundle path
	pool = x509.NewCertPool()
	pemData, err := os.ReadFile(caCertBundlePath)
	if err != nil {
		logger.Fatalf("loadCACertPool: FATAL — cannot read CA certs from %s: %v. TLS will fail. Install ca-certificates package on OpenWrt.", caCertBundlePath, err)
	}
	if !pool.AppendCertsFromPEM(pemData) {
		logger.Fatalf("loadCACertPool: FATAL — no valid CA certificates found in %s. TLS will fail.", caCertBundlePath)
	}

	logger.Infof("loadCACertPool: loaded %d CA cert subjects from %s", len(pool.Subjects()), caCertBundlePath)
	return pool
}

type mintConfigProvider interface {
	GetConfig() *config_manager.Config
}

type MintHealthTracker struct {
	mu                    sync.RWMutex
	reachableMints        map[string]bool
	consecutiveSuccesses  map[string]uint8
	httpClient            *http.Client
	configProvider        mintConfigProvider
	recoveryThreshold     uint8
	onFirstReachable      func()
	hadReachableMint      bool
	onReachableSetChanged func()
	reachableCount        int
	stopCh                chan struct{}
}

func NewMintHealthTracker(configProvider mintConfigProvider) *MintHealthTracker {
	// Build a TLS config that allows TLS 1.2 and 1.3, with explicit CA cert
	// loading for OpenWrt compatibility.
	caPool := loadCACertPool()
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
		RootCAs:    caPool,
	}

	transport := &http.Transport{
		TLSClientConfig:       tlsConfig,
		TLSHandshakeTimeout:   20 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DisableKeepAlives:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		ForceAttemptHTTP2:     false,
	}

	return &MintHealthTracker{
		reachableMints:       make(map[string]bool),
		consecutiveSuccesses: make(map[string]uint8),
		httpClient: &http.Client{
			Timeout:   probeTimeout,
			Transport: transport,
		},
		configProvider:    configProvider,
		recoveryThreshold: defaultRecoveryThreshold,
	}
}

func (t *MintHealthTracker) StartProactiveChecks() {
	t.mu.Lock()
	if t.stopCh != nil {
		t.mu.Unlock()
		return
	}
	t.stopCh = make(chan struct{})
	stopCh := t.stopCh
	needAggressive := t.reachableCount == 0
	t.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("StartProactiveChecks goroutine panicked: %v", r)
			}
		}()

		var aggressiveDone chan struct{}
		if needAggressive {
			logger.Infof("StartProactiveChecks: starting aggressive retry (no reachable mints at startup)")
			aggressiveDone = t.runAggressiveRetry(stopCh)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Errorf("StartProactiveChecks aggressive-done watcher panicked: %v", r)
					}
				}()
				<-aggressiveDone
				logger.Infof("StartProactiveChecks: aggressive retry completed")
			}()
		}

		ticker := time.NewTicker(probeInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				t.runProactiveCheck()
			case <-stopCh:
				return
			}
		}
	}()
}

func (t *MintHealthTracker) runAggressiveRetry(stopCh chan struct{}) chan struct{} {
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Errorf("runAggressiveRetry goroutine panicked: %v", r)
			}
		}()
		defer close(done)
		aggressiveClient := &http.Client{Timeout: aggressiveProbeTimeout}
		ticker := time.NewTicker(aggressiveProbeInterval)
		defer ticker.Stop()
		timer := time.NewTimer(aggressiveDuration)
		defer timer.Stop()

		for {
			select {
			case <-ticker.C:
				if t.runAggressiveCheck(aggressiveClient) {
					logger.Infof("runAggressiveRetry: mint became reachable, stopping aggressive mode")
					return
				}
			case <-timer.C:
				logger.Infof("runAggressiveRetry: aggressive period ended (%v), falling back to normal interval", aggressiveDuration)
				return
			case <-stopCh:
				return
			}
		}
	}()
	return done
}

func (t *MintHealthTracker) Stop() {
	t.mu.Lock()
	if t.stopCh != nil {
		close(t.stopCh)
		t.stopCh = nil
	}
	t.mu.Unlock()
}

func (t *MintHealthTracker) IsReachable(mintURL string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.reachableMints[mintURL]
}

func (t *MintHealthTracker) GetReachableMintConfigs() []config_manager.MintConfig {
	config := t.configProvider.GetConfig()
	if config == nil {
		return nil
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	var reachable []config_manager.MintConfig
	for _, mint := range config.AcceptedMints {
		if t.reachableMints[mint.URL] {
			reachable = append(reachable, mint)
		}
	}
	return reachable
}

func (t *MintHealthTracker) GetAllConfiguredMintConfigs() []config_manager.MintConfig {
	config := t.configProvider.GetConfig()
	if config == nil {
		return nil
	}
	return config.AcceptedMints
}

func (t *MintHealthTracker) MarkUnreachable(mintURL string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.reachableMints[mintURL] {
		t.reachableCount--
	}
	t.reachableMints[mintURL] = false
	t.consecutiveSuccesses[mintURL] = 0
}

// SetOnFirstReachableForDegraded registers a callback that fires once when a mint
// becomes reachable after starting with none. The hadReachableMint flag is reset to
// false so the callback fires on the first mint recovery — this is only meaningful
// for the degraded merchant path which starts with all mints unreachable.
func (t *MintHealthTracker) SetOnFirstReachableForDegraded(callback func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onFirstReachable = callback
	t.hadReachableMint = false
}

func (t *MintHealthTracker) SetOnReachableSetChanged(callback func()) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onReachableSetChanged = callback
}

func (t *MintHealthTracker) RunInitialProbe() {
	config := t.configProvider.GetConfig()
	if config == nil {
		return
	}

	mints := config.AcceptedMints
	logger.Infof("RunInitialProbe: probing %d mint(s) with TLS config: min=%s max=%s", len(mints), "TLS 1.2", "TLS 1.3")

	for round := 1; round <= initialProbeMaxRetries; round++ {
		results := make(map[string]bool, len(mints))
		for _, mint := range mints {
			results[mint.URL] = t.probeMint(mint.URL)
		}

		// Check if any mint is reachable
		anyReachable := false
		for _, ok := range results {
			if ok {
				anyReachable = true
				break
			}
		}

		if anyReachable {
			if round > 1 {
				logger.Infof("RunInitialProbe: mints reachable on retry round %d/%d", round, initialProbeMaxRetries)
			}
			break
		}

		if round < initialProbeMaxRetries {
			logger.Warnf("RunInitialProbe: all mints unreachable on round %d/%d, retrying in %v", round, initialProbeMaxRetries, initialProbeRetryDelay)
			time.Sleep(initialProbeRetryDelay)
		} else {
			logger.Errorf("RunInitialProbe: all mints unreachable after %d retry rounds", initialProbeMaxRetries)
		}
	}

	// Re-probe after retries to get final results
	results := make(map[string]bool, len(mints))
	for _, mint := range mints {
		results[mint.URL] = t.probeMint(mint.URL)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	for url, ok := range results {
		if ok {
			t.reachableMints[url] = true
			t.consecutiveSuccesses[url] = t.recoveryThreshold
		} else {
			t.reachableMints[url] = false
			t.consecutiveSuccesses[url] = 0
		}
	}

	t.reachableCount = 0
	for _, mint := range mints {
		if t.reachableMints[mint.URL] {
			t.hadReachableMint = true
			t.reachableCount++
		}
	}
}

func (t *MintHealthTracker) RunProactiveCheck() {
	t.runProactiveCheck()
}

func (t *MintHealthTracker) runProactiveCheck() {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("runProactiveCheck panicked: %v", r)
		}
	}()

	config := t.configProvider.GetConfig()
	if config == nil {
		return
	}

	logger.Infof("runProactiveCheck: probing %d mint(s)", len(config.AcceptedMints))
	results := make(map[string]bool, len(config.AcceptedMints))
	for _, mint := range config.AcceptedMints {
		results[mint.URL] = t.probeMint(mint.URL)
	}

	t.mu.Lock()

	for _, mint := range config.AcceptedMints {
		if results[mint.URL] {
			t.consecutiveSuccesses[mint.URL]++

			if !t.reachableMints[mint.URL] && t.consecutiveSuccesses[mint.URL] >= t.recoveryThreshold {
				t.reachableMints[mint.URL] = true
			}
		} else {
			t.consecutiveSuccesses[mint.URL] = 0
			t.reachableMints[mint.URL] = false
		}
	}

	newCount := 0
	for _, mint := range config.AcceptedMints {
		if t.reachableMints[mint.URL] {
			newCount++
		}
	}

	setChanged := newCount != t.reachableCount
	t.reachableCount = newCount

	var callbacks []func()

	if !t.hadReachableMint && t.onFirstReachable != nil {
		for _, mint := range config.AcceptedMints {
			if t.reachableMints[mint.URL] {
				t.hadReachableMint = true
				callbacks = append(callbacks, t.onFirstReachable)
				break
			}
		}
	}

	if setChanged && t.onReachableSetChanged != nil {
		callbacks = append(callbacks, t.onReachableSetChanged)
	}

	t.mu.Unlock()

	for _, cb := range callbacks {
		logger.Infof("runProactiveCheck: firing callback (hadReachable=%v, setChanged=%v)", t.hadReachableMint, setChanged)
		go cb()
	}
}

// runAggressiveCheck probes mints with immediate recovery (threshold=1).
// Returns true if a previously-unreachable mint became reachable.
func (t *MintHealthTracker) runAggressiveCheck(aggressiveClient *http.Client) bool {
	config := t.configProvider.GetConfig()
	if config == nil {
		return false
	}

	logger.Infof("runAggressiveCheck: probing %d mint(s) with immediate recovery", len(config.AcceptedMints))
	results := make(map[string]bool, len(config.AcceptedMints))
	for _, mint := range config.AcceptedMints {
		results[mint.URL] = t.probeMintWith(mint.URL, aggressiveClient)
	}

	t.mu.Lock()

	recovered := false
	for _, mint := range config.AcceptedMints {
		if results[mint.URL] {
			t.consecutiveSuccesses[mint.URL]++
			if !t.reachableMints[mint.URL] {
				t.reachableMints[mint.URL] = true
				recovered = true
			}
		} else {
			t.consecutiveSuccesses[mint.URL] = 0
			t.reachableMints[mint.URL] = false
		}
	}

	newCount := 0
	for _, mint := range config.AcceptedMints {
		if t.reachableMints[mint.URL] {
			newCount++
		}
	}

	setChanged := newCount != t.reachableCount
	t.reachableCount = newCount

	var callbacks []func()

	if !t.hadReachableMint && t.onFirstReachable != nil {
		for _, mint := range config.AcceptedMints {
			if t.reachableMints[mint.URL] {
				t.hadReachableMint = true
				callbacks = append(callbacks, t.onFirstReachable)
				break
			}
		}
	}

	if setChanged && t.onReachableSetChanged != nil {
		callbacks = append(callbacks, t.onReachableSetChanged)
	}

	t.mu.Unlock()

	for _, cb := range callbacks {
		logger.Infof("runAggressiveCheck: firing callback (hadReachable=%v, setChanged=%v)", t.hadReachableMint, setChanged)
		go cb()
	}

	return recovered
}

func (t *MintHealthTracker) probeMint(mintURL string) bool {
	return t.probeMintWith(mintURL, t.httpClient)
}

func (t *MintHealthTracker) probeMintWith(mintURL string, client *http.Client) bool {
	url := strings.TrimRight(mintURL, "/") + "/v1/info"

	start := time.Now()
	resp, err := client.Get(url)
	elapsed := time.Since(start)
	if err != nil {
		logger.Errorf("mint probe FAILED: url=%s elapsed=%s error=%v", url, elapsed, err)
		return false
	}
	defer resp.Body.Close()

	ok := resp.StatusCode >= 200 && resp.StatusCode < 300
	logger.Infof("mint probe: url=%s status=%d elapsed=%s ok=%v", url, resp.StatusCode, elapsed, ok)
	return ok
}
